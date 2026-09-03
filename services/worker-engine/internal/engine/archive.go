package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"coldharbor/worker-engine/internal/model"
	"github.com/redis/go-redis/v9"
)

// ArchiveManager handles sealing immutable final results in Redis as Dead Drops with SHA-256 and TTL
type ArchiveManager struct {
	client     redis.UniversalClient
	defaultTTL time.Duration
}

// NewArchiveManager creates an archive manager with client and default TTL
func NewArchiveManager(client redis.UniversalClient, defaultTTL time.Duration) *ArchiveManager {
	if defaultTTL <= 0 {
		defaultTTL = 3600 * time.Second
	}
	return &ArchiveManager{
		client:     client,
		defaultTTL: defaultTTL,
	}
}

// Key returns the Redis key for the Dead Drop archive
func (a *ArchiveManager) Key(compartmentID string) string {
	return fmt.Sprintf("archive:%s", compartmentID)
}

// ComputeChecksum calculates deterministic SHA-256 hex string of the given payload
func ComputeChecksum(data any) (string, error) {
	var bytes []byte
	switch v := data.(type) {
	case nil:
		bytes = []byte("")
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("failed to marshal data for checksum: %w", err)
		}
		bytes = b
	}

	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:]), nil
}

// Seal stores the sealed Dead Drop payload in Redis string key with SHA-256 checksum and TTL
func (a *ArchiveManager) Seal(ctx context.Context, dd *model.DeadDropPayload) error {
	if dd == nil || dd.CompartmentID == "" {
		return fmt.Errorf("invalid dead drop payload: compartment ID is required")
	}

	// Calculate checksum from Output or ResultPayload if not already set
	if dd.Checksum == "" {
		var targetData any
		if dd.Output != nil {
			targetData = dd.Output
		} else if dd.ResultPayload != nil {
			targetData = dd.ResultPayload
		}
		checksum, err := ComputeChecksum(targetData)
		if err != nil {
			return fmt.Errorf("failed to compute checksum: %w", err)
		}
		dd.Checksum = checksum
	}

	now := time.Now().UTC()
	if dd.ArchivedAt.IsZero() {
		dd.ArchivedAt = now
	}
	if dd.CompletedAt.IsZero() {
		dd.CompletedAt = now
	}

	ttl := a.defaultTTL
	if dd.TTLSeconds > 0 {
		ttl = time.Duration(dd.TTLSeconds) * time.Second
	} else {
		dd.TTLSeconds = int(ttl.Seconds())
	}

	// Normalize Output and ResultPayload for contract compatibility
	if dd.ResultPayload == nil && dd.Output != nil {
		dd.ResultPayload = dd.Output
	} else if dd.Output == nil && dd.ResultPayload != nil {
		if outMap, ok := dd.ResultPayload.(map[string]any); ok {
			dd.Output = outMap
		}
	}

	data, err := json.Marshal(dd)
	if err != nil {
		return fmt.Errorf("failed to marshal dead drop: %w", err)
	}

	key := a.Key(dd.CompartmentID)
	if err := a.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("failed to write dead drop to %s: %w", key, err)
	}

	return nil
}

// Get retrieves the sealed Dead Drop payload from Redis
func (a *ArchiveManager) Get(ctx context.Context, compartmentID string) (*model.DeadDropPayload, error) {
	key := a.Key(compartmentID)
	val, err := a.client.Get(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve dead drop %s: %w", key, err)
	}

	var dd model.DeadDropPayload
	if err := json.Unmarshal([]byte(val), &dd); err != nil {
		return nil, fmt.Errorf("failed to unmarshal dead drop payload: %w", err)
	}

	return &dd, nil
}

// VerifyChecksum validates that the stored checksum matches the payload data
func (a *ArchiveManager) VerifyChecksum(dd *model.DeadDropPayload) (bool, error) {
	if dd == nil {
		return false, fmt.Errorf("dead drop is nil")
	}

	var target any
	if dd.Output != nil {
		target = dd.Output
	} else if dd.ResultPayload != nil {
		target = dd.ResultPayload
	}

	expected, err := ComputeChecksum(target)
	if err != nil {
		return false, err
	}

	return dd.Checksum == expected, nil
}
