package engine

import (
	"context"
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
			switch v := val.(type) {
			case float64:
				crashAtStep = int(v)
			case int:
				crashAtStep = v
			}
		}
		if val, ok := job.Payload["simulateFailure"]; ok {
			if fail, ok := val.(bool); ok && fail {
				return nil, fmt.Errorf("task computation execution error")
			}
		}
	}

	// Internal state accumulator
	var accumulator int64 = 0
	if intermediateData != "" {
		var state map[string]any
		if err := json.Unmarshal([]byte(intermediateData), &state); err == nil {
			if v, ok := state["accumulator"].(float64); ok {
				accumulator = int64(v)
			}
		}
	}

	// Extract batch parameters
	batchSize := int64(500)
	if job.Payload != nil {
		if bVal, ok := job.Payload["batchSize"]; ok {
			if bFloat, ok := bVal.(float64); ok {
				batchSize = int64(bFloat)
			}
		}
	}

	// Step 1: 25% milestone
	if currentStep < 1 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		accumulator += batchSize * 10
		if err := recorder.Record(ctx, 1, 25, map[string]any{
			"step":        1,
			"accumulator": accumulator,
			"milestone":   "first_quartile_reduced",
		}); err != nil {
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
		accumulator += batchSize * 25
		if err := recorder.Record(ctx, 2, 50, map[string]any{
			"step":        2,
			"accumulator": accumulator,
			"milestone":   "half_quartile_reduced",
		}); err != nil {
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
		accumulator += batchSize * 42
		if err := recorder.Record(ctx, 3, 75, map[string]any{
			"step":        3,
			"accumulator": accumulator,
			"milestone":   "third_quartile_reduced",
		}); err != nil {
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
	accumulator += batchSize*21 + 201

	output := map[string]any{
		"processedCount": batchSize,
		"reducedSum":     accumulator,
		"status":         "SUCCESS",
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
