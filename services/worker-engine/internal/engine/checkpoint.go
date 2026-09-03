package engine

import (
	"context"
	"fmt"

	"coldharbor/worker-engine/internal/model"
)

// CheckpointRecorder manages writing progress milestones (25%, 50%, 75%)
type CheckpointRecorder struct {
	scratchpad *ScratchpadManager
	fsm        *FSM
}

// NewCheckpointRecorder creates a new checkpoint recorder
func NewCheckpointRecorder(scratchpad *ScratchpadManager, fsm *FSM) *CheckpointRecorder {
	return &CheckpointRecorder{
		scratchpad: scratchpad,
		fsm:        fsm,
	}
}

// Record saves checkpoint data to the ephemeral scratchpad, transitions the FSM to CHECKPOINT,
// emits a pub/sub event, and transitions back to RUNNING.
func (c *CheckpointRecorder) Record(ctx context.Context, step int, pct int, intermediate any) error {
	compartmentID := c.fsm.compartmentID

	// 1. Transition FSM: RUNNING -> CHECKPOINT
	if err := c.fsm.Transition(ctx, model.StateCheckpoint, pct, fmt.Sprintf("Checkpoint %d%% recorded", pct)); err != nil {
		return fmt.Errorf("failed to transition to CHECKPOINT at %d%%: %w", pct, err)
	}

	// 2. Persist step and intermediate state to scratchpad hash
	if err := c.scratchpad.SetStep(ctx, compartmentID, step, pct, intermediate); err != nil {
		return fmt.Errorf("failed to persist checkpoint %d%% to scratchpad: %w", pct, err)
	}

	// 3. Transition FSM: CHECKPOINT -> RUNNING to continue execution
	if err := c.fsm.Transition(ctx, model.StateRunning, pct, fmt.Sprintf("Resumed execution after checkpoint %d%%", pct)); err != nil {
		return fmt.Errorf("failed to resume RUNNING after checkpoint %d%%: %w", pct, err)
	}

	return nil
}
