package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"coldharbor/worker-engine/internal/model"
)

// TaskResult holds the final output and execution duration
type TaskResult struct {
	Output     map[string]any
	DurationMs int64
}

// TaskExecutor allows the consumer to execute jobs while tests can inject a
// deterministic, race-safe executor.
type TaskExecutor interface {
	Execute(context.Context, *model.JobMessage, *CheckpointRecorder) (*TaskResult, error)
}

// TaskRunner orchestrates multi-step task execution, checkpointing, and resume logic
type TaskRunner struct {
	scratchpad *ScratchpadManager
}

// NewTaskRunner creates a new task runner
func NewTaskRunner(scratchpad *ScratchpadManager) *TaskRunner {
	return &TaskRunner{
		scratchpad: scratchpad,
	}
}

// Execute runs the task through its lifecycle steps (25%, 50%, 75%, 100%),
// checking for existing checkpoints to resume work without re-executing earlier steps.
func (r *TaskRunner) Execute(
	ctx context.Context,
	job *model.JobMessage,
	recorder *CheckpointRecorder,
) (*TaskResult, error) {
	start := time.Now()
	compartmentID := job.CompartmentID

	// Check if this job is being resumed from a previous crash/checkpoint
	currentStep, _, intermediateData, err := r.scratchpad.GetStep(ctx, compartmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing checkpoint: %w", err)
	}

	// Check for crash simulation flag in payload (used in E2E crash-recovery verification)
	crashAtStep := 0
	if job.Payload != nil {
		if val, ok := job.Payload["simulateCrashAtStep"]; ok {
			if v, ok := numericInt64(val); ok {
				crashAtStep = int(v)
			}
		}
		if val, ok := job.Payload["simulateFailure"]; ok {
			if fail, ok := val.(bool); ok && fail {
				return nil, fmt.Errorf("task computation execution error")
			}
		}
	}

	// Internal state accumulator (DATA_REDUCTION) plus per-type resume state.
	// Each task type uses distinct CPU-bound work so the Go worker pool earns
	// its place: numeric reduction vs. hash-chain stream vs. seal canonicalization.
	taskType := job.TaskType
	if taskType == "" {
		taskType = "DATA_REDUCTION"
	}
	var accumulator int64 = 0
	var streamDigest string
	var sealedCount int64
	if intermediateData != "" {
		var state map[string]any
		if err := json.Unmarshal([]byte(intermediateData), &state); err == nil {
			if v, ok := state["accumulator"].(float64); ok {
				accumulator = int64(v)
			}
			if v, ok := state["digest"].(string); ok {
				streamDigest = v
			}
			if v, ok := state["sealed"].(float64); ok {
				sealedCount = int64(v)
			}
		}
	}

	// Extract batch parameters
	batchSize := int64(500)
	if job.Payload != nil {
		if bVal, ok := job.Payload["batchSize"]; ok {
			if value, ok := numericInt64(bVal); ok {
				batchSize = value
			}
		}
	}
	inputSeed := inputFingerprint(job.Payload)
	// Step 1: 25% milestone
	if currentStep < 1 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var state map[string]any
		switch taskType {
		case "CIPHER_STREAM":
			streamDigest = chainDigest(streamDigest, inputSeed, 1)
			state = map[string]any{"step": 1, "digest": streamDigest, "milestone": "stream_quartile_1"}
		case "ARCHIVE_SEAL":
			sealedCount += batchSize / 4
			state = map[string]any{"step": 1, "sealed": sealedCount, "milestone": "seal_quartile_1"}
		default:
			accumulator += batchSize * 10
			state = map[string]any{"step": 1, "accumulator": accumulator, "milestone": "first_quartile_reduced"}
		}
		if err := recorder.Record(ctx, 1, 25, state); err != nil {
			return nil, fmt.Errorf("step 1 failed: %w", err)
		}
		if crashAtStep == 1 {
			return nil, fmt.Errorf("SIMULATED_WORKER_CRASH at step 1 (25%%)")
		}
	}

	// Step 2: 50% milestone
	if currentStep < 2 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var state map[string]any
		switch taskType {
		case "CIPHER_STREAM":
			streamDigest = chainDigest(streamDigest, inputSeed, 2)
			state = map[string]any{"step": 2, "digest": streamDigest, "milestone": "stream_quartile_2"}
		case "ARCHIVE_SEAL":
			sealedCount += batchSize / 4
			state = map[string]any{"step": 2, "sealed": sealedCount, "milestone": "seal_quartile_2"}
		default:
			accumulator += batchSize * 25
			state = map[string]any{"step": 2, "accumulator": accumulator, "milestone": "half_quartile_reduced"}
		}
		if err := recorder.Record(ctx, 2, 50, state); err != nil {
			return nil, fmt.Errorf("step 2 failed: %w", err)
		}
		if crashAtStep == 2 {
			return nil, fmt.Errorf("SIMULATED_WORKER_CRASH at step 2 (50%%)")
		}
	}

	// Step 3: 75% milestone
	if currentStep < 3 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		var state map[string]any
		switch taskType {
		case "CIPHER_STREAM":
			streamDigest = chainDigest(streamDigest, inputSeed, 3)
			state = map[string]any{"step": 3, "digest": streamDigest, "milestone": "stream_quartile_3"}
		case "ARCHIVE_SEAL":
			sealedCount += batchSize / 4
			state = map[string]any{"step": 3, "sealed": sealedCount, "milestone": "seal_quartile_3"}
		default:
			accumulator += batchSize * 42
			state = map[string]any{"step": 3, "accumulator": accumulator, "milestone": "third_quartile_reduced"}
		}
		if err := recorder.Record(ctx, 3, 75, state); err != nil {
			return nil, fmt.Errorf("step 3 failed: %w", err)
		}
		if crashAtStep == 3 {
			return nil, fmt.Errorf("SIMULATED_WORKER_CRASH at step 3 (75%%)")
		}
	}

	// Step 4: Final 100% completion
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	output := map[string]any{"processedCount": batchSize, "status": "SUCCESS"}
	switch taskType {
	case "CIPHER_STREAM":
		streamDigest = chainDigest(streamDigest, inputSeed, 4)
		output["streamDigest"] = streamDigest
	case "ARCHIVE_SEAL":
		sealedCount += batchSize - 3*(batchSize/4)
		sum := sha256.Sum256([]byte(inputSeed))
		output["sealedCount"] = sealedCount
		output["sealChecksum"] = hex.EncodeToString(sum[:])
	default:
		accumulator += batchSize*21 + 201
		output["reducedSum"] = accumulator
	}

	duration := time.Since(start).Milliseconds()
	if duration == 0 {
		duration = 1
	}

	return &TaskResult{
		Output:     output,
		DurationMs: duration,
	}, nil
}

func chainDigest(prev, seed string, step int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d", prev, seed, step)))
	return hex.EncodeToString(sum[:])
}

func inputFingerprint(payload map[string]any) string {
	if payload == nil {
		return "empty"
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("%v", payload)
	}
	return string(b)
}
