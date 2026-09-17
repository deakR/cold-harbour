package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	FieldCurrentStep  = "current_step"
	FieldProgressPct  = "progress_pct"
	FieldIntermediate = "intermediate_result"
	FieldLastUpdated  = "last_updated"
)

// ScratchpadManager handles ephemeral Redis Hash storage for compartment computation state
type ScratchpadManager struct {
	client redis.UniversalClient
}

// NewScratchpadManager initializes a scratchpad manager with a Redis client
func NewScratchpadManager(client redis.UniversalClient) *ScratchpadManager {
	return &ScratchpadManager{client: client}
}

// Key returns the canonical Redis key for a compartment's scratchpad memory
func (s *ScratchpadManager) Key(compartmentID string) string {
	return fmt.Sprintf("compartment:%s:mem", compartmentID)
}

// Write sets arbitrary key-value pairs in the compartment's scratchpad hash
func (s *ScratchpadManager) Write(ctx context.Context, compartmentID string, data map[string]any) error {
	if len(data) == 0 {
		return nil
	}

	fields := make(map[string]any, len(data)+1)
	for k, v := range data {
		switch val := v.(type) {
		case string:
			fields[k] = val
		case []byte:
			fields[k] = string(val)
		default:
			b, err := json.Marshal(val)
			if err != nil {
				return fmt.Errorf("failed to marshal scratchpad field %s: %w", k, err)
			}
			fields[k] = string(b)
		}
	}
	fields[FieldLastUpdated] = time.Now().UTC().Format(time.RFC3339Nano)

	key := s.Key(compartmentID)
	if err := s.client.HSet(ctx, key, fields).Err(); err != nil {
		return fmt.Errorf("failed to write to scratchpad %s: %w", key, err)
	}
	return nil
}

// Read returns all fields from the compartment's scratchpad hash
func (s *ScratchpadManager) Read(ctx context.Context, compartmentID string) (map[string]string, error) {
	key := s.Key(compartmentID)
	res, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to read scratchpad %s: %w", key, err)
	}
	return res, nil
}

// SetStep records an execution checkpoint step and intermediate result in the scratchpad
func (s *ScratchpadManager) SetStep(ctx context.Context, compartmentID string, step int, pct int, intermediate any) error {
	intermediateStr := ""
	if intermediate != nil {
		switch val := intermediate.(type) {
		case string:
			intermediateStr = val
		case []byte:
			intermediateStr = string(val)
		default:
			b, err := json.Marshal(val)
			if err != nil {
				return fmt.Errorf("failed to marshal intermediate data: %w", err)
			}
			intermediateStr = string(b)
		}
	}

	data := map[string]any{
		FieldCurrentStep:  step,
		FieldProgressPct:  pct,
		FieldIntermediate: intermediateStr,
	}

	return s.Write(ctx, compartmentID, data)
}

// GetStep reads the latest checkpoint step and intermediate result from the scratchpad
func (s *ScratchpadManager) GetStep(ctx context.Context, compartmentID string) (step int, pct int, intermediate string, err error) {
	key := s.Key(compartmentID)
	fields, err := s.client.HMGet(ctx, key, FieldCurrentStep, FieldProgressPct, FieldIntermediate).Result()
	if err != nil {
		return 0, 0, "", fmt.Errorf("failed to retrieve scratchpad step from %s: %w", key, err)
	}

	if len(fields) < 3 || fields[0] == nil {
		return 0, 0, "", nil // Not initialized or empty
	}

	if sStr, ok := fields[0].(string); ok && sStr != "" {
		step, _ = strconv.Atoi(sStr)
	}
	if pStr, ok := fields[1].(string); ok && pStr != "" {
		pct, _ = strconv.Atoi(pStr)
	}
	if iStr, ok := fields[2].(string); ok {
		intermediate = iStr
	}

	return step, pct, intermediate, nil
}

// Purge executes an atomic DEL on the compartment's scratchpad hash.
// Zero-leak invariant: guarantees compartment:{id}:mem does not exist post-call.
func (s *ScratchpadManager) Purge(ctx context.Context, compartmentID string) error {
	key := s.Key(compartmentID)
	if err := s.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to purge scratchpad %s: %w", key, err)
	}
	return nil
}

// Exists checks if the compartment's scratchpad hash key exists in Redis
func (s *ScratchpadManager) Exists(ctx context.Context, compartmentID string) (bool, error) {
	key := s.Key(compartmentID)
	count, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check existence of %s: %w", key, err)
	}
	return count > 0, nil
}
