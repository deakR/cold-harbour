package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"coldharbour/internal/checkpoint"
	"coldharbour/internal/journal"
	"coldharbour/internal/redact"
	"coldharbour/internal/seal"

	"github.com/redis/go-redis/v9"
)

const (
	memFieldStep    = "step"
	memFieldPartial = "partial_result"
	memFieldStep1   = "step1"
)

type memHash struct {
	rdb  *redis.Client
	keys seal.KeyStore
}

func memKey(jobID string) string {
	return "job:" + jobID + ":mem"
}

func (m *memHash) jobKey(ctx context.Context, jobID string) ([32]byte, error) {
	return m.keys.Ensure(ctx, journal.DurableIDFor(jobID))
}

func (m *memHash) load(ctx context.Context, jobID string) (*checkpoint.Checkpoint, error) {
	fields, err := m.rdb.HGetAll(ctx, memKey(jobID)).Result()
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
	}
	stepRaw := fields[memFieldStep]
	if stepRaw == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(stepRaw)
	if err != nil {
		return nil, fmt.Errorf("mem hash step: %w", err)
	}
	raw := fields[memFieldPartial]
	if raw == "" {
		return nil, fmt.Errorf("mem hash missing partial_result")
	}
	key, err := m.jobKey(ctx, jobID)
	if err != nil {
		return nil, err
	}
	plain, err := seal.Open(key, raw)
	if err != nil {
		return nil, fmt.Errorf("mem hash partial_result: %w", err)
	}
	var partial redact.RedactResult
	if err := json.Unmarshal(plain, &partial); err != nil {
		return nil, fmt.Errorf("mem hash partial_result: %w", err)
	}
	return &checkpoint.Checkpoint{
		JobID:         jobID,
		Step:          checkpoint.Step(n),
		PartialResult: partial,
	}, nil
}

func (m *memHash) sealPartial(ctx context.Context, cp checkpoint.Checkpoint) (string, error) {
	partial, err := json.Marshal(cp.PartialResult)
	if err != nil {
		return "", err
	}
	key, err := m.jobKey(ctx, cp.JobID)
	if err != nil {
		return "", err
	}
	return seal.Seal(key, partial)
}

func (m *memHash) save(ctx context.Context, cp checkpoint.Checkpoint) error {
	sealed, err := m.sealPartial(ctx, cp)
	if err != nil {
		return err
	}
	return m.rdb.HSet(ctx, memKey(cp.JobID), memFieldStep, int(cp.Step), memFieldPartial, sealed).Err()
}

func (m *memHash) saveStep1(ctx context.Context, cp checkpoint.Checkpoint) error {
	sealed, err := m.sealPartial(ctx, cp)
	if err != nil {
		return err
	}
	key := memKey(cp.JobID)
	pipe := m.rdb.TxPipeline()
	pipe.HIncrBy(ctx, key, memFieldStep1, 1)
	pipe.HSet(ctx, key, memFieldStep, int(cp.Step), memFieldPartial, sealed)
	_, err = pipe.Exec(ctx)
	return err
}
