package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// RecoveryManager handles PEL (Pending Entries List) reclamation and crash resumption
type RecoveryManager struct {
	client      redis.UniversalClient
	streamName  string
	groupName   string
	workerID    string
	minIdleTime time.Duration
	scratchpad  *ScratchpadManager
}

// NewRecoveryManager creates a recovery manager for reclaiming abandoned jobs
func NewRecoveryManager(
	client redis.UniversalClient,
	streamName, groupName, workerID string,
	minIdleTime time.Duration,
	scratchpad *ScratchpadManager,
) *RecoveryManager {
	if minIdleTime <= 0 {
		minIdleTime = 30 * time.Second
	}
	return &RecoveryManager{
		client:      client,
		streamName:  streamName,
		groupName:   groupName,
		workerID:    workerID,
		minIdleTime: minIdleTime,
		scratchpad:  scratchpad,
	}
}

// ClaimPendingJobs attempts to reclaim unacknowledged messages idle longer than minIdleTime.
// Uses XAUTOCLAIM with fallback to XPENDING + XCLAIM for maximum Redis compatibility.
func (r *RecoveryManager) ClaimPendingJobs(ctx context.Context, count int64) ([]redis.XMessage, error) {
	if count <= 0 {
		count = 10
	}

	// 1. Try XAutoClaim (Redis 6.2+)
	claimedMsgs, _, err := r.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   r.streamName,
		Group:    r.groupName,
		Consumer: r.workerID,
		MinIdle:  r.minIdleTime,
		Start:    "0-0",
		Count:    count,
	}).Result()

	if err == nil {
		return claimedMsgs, nil
	}

	// 2. Fallback to XPendingExt + XClaim if XAutoClaim is unsupported
	if strings.Contains(strings.ToLower(err.Error()), "unknown command") ||
		strings.Contains(strings.ToLower(err.Error()), "err unknown") {
		pendings, pErr := r.client.XPendingExt(ctx, &redis.XPendingExtArgs{
			Stream: r.streamName,
			Group:  r.groupName,
			Start:  "-",
			End:    "+",
			Count:  count,
			Idle:   r.minIdleTime,
		}).Result()
		if pErr != nil {
			return nil, fmt.Errorf("fallback XPendingExt failed: %w", pErr)
		}

		if len(pendings) == 0 {
			return nil, nil
		}

		msgIDs := make([]string, 0, len(pendings))
		for _, p := range pendings {
			msgIDs = append(msgIDs, p.ID)
		}

		claimed, cErr := r.client.XClaim(ctx, &redis.XClaimArgs{
			Stream:   r.streamName,
			Group:    r.groupName,
			Consumer: r.workerID,
			MinIdle:  r.minIdleTime,
			Messages: msgIDs,
		}).Result()
		if cErr != nil {
			return nil, fmt.Errorf("fallback XClaim failed: %w", cErr)
		}

		return claimed, nil
	}

	return nil, fmt.Errorf("failed to reclaim pending jobs via XAutoClaim: %w", err)
}

// InspectCheckpoint queries the scratchpad to determine what step was last saved
func (r *RecoveryManager) InspectCheckpoint(ctx context.Context, compartmentID string) (step int, pct int, intermediate string, err error) {
	return r.scratchpad.GetStep(ctx, compartmentID)
}
