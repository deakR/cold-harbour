package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"coldharbor/worker-engine/internal/model"
	"github.com/redis/go-redis/v9"
)

// DLQHandler manages forwarding failed jobs to the Dead-Letter Queue and purging transient state
type DLQHandler struct {
	client      redis.UniversalClient
	dlqStream   string
	auditStream string
	scratchpad  *ScratchpadManager
	publisher   EventPublisher
}

// NewDLQHandler creates a new DLQ handler
func NewDLQHandler(
	client redis.UniversalClient,
	dlqStream string,
	scratchpad *ScratchpadManager,
	publisher EventPublisher,
	auditStream ...string,
) *DLQHandler {
	if dlqStream == "" {
		dlqStream = "coldharbor:jobs:dlq"
	}
	auditStreamName := "coldharbor:audits"
	if len(auditStream) > 0 && auditStream[0] != "" {
		auditStreamName = auditStream[0]
	}
	return &DLQHandler{
		client:      client,
		dlqStream:   dlqStream,
		auditStream: auditStreamName,
		scratchpad:  scratchpad,
		publisher:   publisher,
	}
}

// RetryKey returns the Redis key used to track retry attempts for a compartment
func (d *DLQHandler) RetryKey(compartmentID string) string {
	return fmt.Sprintf("coldharbor:retries:%s", compartmentID)
}

// GetRetryCount returns the current retry count for a compartment
func (d *DLQHandler) GetRetryCount(ctx context.Context, compartmentID string) (int, error) {
	val, err := d.client.Get(ctx, d.RetryKey(compartmentID)).Int()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get retry count: %w", err)
	}
	return val, nil
}

// IncrementRetryCount increments and returns the retry counter for a compartment with a 24h expiration
func (d *DLQHandler) IncrementRetryCount(ctx context.Context, compartmentID string) (int, error) {
	key := d.RetryKey(compartmentID)
	pipe := d.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, 24*time.Hour)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to increment retry count: %w", err)
	}
	return int(incr.Val()), nil
}

// RouteToDLQ writes the failed job to the DLQ stream, emits FAILED & PURGED events,
// strictly purges the ephemeral scratchpad, and acknowledges the original stream message.
func (d *DLQHandler) RouteToDLQ(
	ctx context.Context,
	origStream, origGroup, msgID, workerID string,
	job *model.JobMessage,
	attemptCount int,
	reason string,
) error {
	compartmentID := job.CompartmentID

	jobBytes, _ := json.Marshal(job)

	// 1. Add to DLQ stream
	dlqValues := map[string]any{
		"compartmentId": compartmentID,
		"workerId":      workerID,
		"attempts":      attemptCount,
		"reason":        reason,
		"failedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"originalMsgId": msgID,
		"jobData":       string(jobBytes),
	}

	if err := d.client.XAdd(ctx, &redis.XAddArgs{
		Stream: d.dlqStream,
		Values: dlqValues,
	}).Err(); err != nil {
		return fmt.Errorf("failed to write job to DLQ %s: %w", d.dlqStream, err)
	}

	// 2. Publish FAILED event
	if d.publisher != nil {
		if err := d.publisher.Publish(ctx, &model.EventMessage{
			CompartmentID: compartmentID,
			WorkerID:      workerID,
			FromState:     model.StateRunning,
			ToState:       model.StateFailed,
			PreviousState: model.StateRunning,
			CurrentState:  model.StateFailed,
			CheckpointPct: 0,
			Timestamp:     time.Now().UTC(),
			Details:       fmt.Sprintf("Moved to DLQ after %d attempts: %s", attemptCount, reason),
		}); err != nil {
			log.Printf("[dlq] failed to publish FAILED event for compartment %s: %v", compartmentID, err)
		}
	}

	// 3. Purge scratchpad (zero leak guarantee)
	if d.scratchpad != nil {
		if err := d.scratchpad.Purge(ctx, compartmentID); err != nil {
			return fmt.Errorf("failed to purge scratchpad on DLQ routing: %w", err)
		}
		exists, err := d.scratchpad.Exists(ctx, compartmentID)
		if err != nil || exists {
			return fmt.Errorf("zero-leak invariant violated: scratchpad for %s still exists post-purge", compartmentID)
		}
	}

	now := time.Now().UTC()
	if err := appendAuditEnvelope(ctx, d.client, d.auditStream, &model.DeadDropPayload{
		CompartmentID: compartmentID,
		OwnerID:       job.OwnerID,
		Context:       job.Context,
		TaskType:      job.TaskType,
		ArchivedAt:    now,
		CompletedAt:   now,
	}, string(model.StateFailed), reason); err != nil {
		return fmt.Errorf("persist failed-job audit envelope: %w", err)
	}

	// 4. Publish PURGED event
	if d.publisher != nil {
		if err := d.publisher.Publish(ctx, &model.EventMessage{
			CompartmentID: compartmentID,
			WorkerID:      workerID,
			FromState:     model.StateFailed,
			ToState:       model.StatePurged,
			PreviousState: model.StateFailed,
			CurrentState:  model.StatePurged,
			CheckpointPct: 0,
			Timestamp:     time.Now().UTC(),
			Details:       "Scratchpad purged on DLQ routing",
		}); err != nil {
			log.Printf("[dlq] failed to publish PURGED event for compartment %s: %v", compartmentID, err)
		}
	}

	// 5. Clean up retry key
	if err := d.client.Del(ctx, d.RetryKey(compartmentID)).Err(); err != nil {
		log.Printf("[dlq] failed to clear retry key for compartment %s: %v", compartmentID, err)
	}

	// 6. Acknowledge original message from stream
	if origStream != "" && origGroup != "" && msgID != "" {
		if err := d.client.XAck(ctx, origStream, origGroup, msgID).Err(); err != nil {
			return fmt.Errorf("failed to XACK original message %s after DLQ: %w", msgID, err)
		}
	}

	return nil
}
