package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
)

const (
	memFieldStep    = "step"
	memFieldPartial = "partial_result"
	memFieldStep1   = "step1"
)

type memHash struct {
	rdb *redis.Client
}

func memKey(jobID string) string {
	return "job:" + jobID + ":mem"
}

func (m *memHash) load(ctx context.Context, jobID string) (*Checkpoint, error) {
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
	var partial RedactResult
	if err := json.Unmarshal([]byte(raw), &partial); err != nil {
		return nil, fmt.Errorf("mem hash partial_result: %w", err)
	}
	return &Checkpoint{
		JobID:         jobID,
		Step:          Step(n),
		PartialResult: partial,
	}, nil
}

func (m *memHash) save(ctx context.Context, cp Checkpoint) error {
	partial, err := json.Marshal(cp.PartialResult)
	if err != nil {
		return err
	}
	return m.rdb.HSet(ctx, memKey(cp.JobID), memFieldStep, int(cp.Step), memFieldPartial, string(partial)).Err()
}

func (m *memHash) incrStep1(ctx context.Context, jobID string) (int, error) {
	n, err := m.rdb.HIncrBy(ctx, memKey(jobID), memFieldStep1, 1).Result()
	return int(n), err
}
