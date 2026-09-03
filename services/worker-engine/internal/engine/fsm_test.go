package engine

import (
	"context"
	"testing"

	"coldharbor/worker-engine/internal/model"
)

type mockPublisher struct {
	events []*model.EventMessage
}

func (m *mockPublisher) Publish(ctx context.Context, event *model.EventMessage) error {
	m.events = append(m.events, event)
	return nil
}

func TestFSMTransitions(t *testing.T) {
	ctx := context.Background()
	pub := &mockPublisher{}
	fsm := NewFSM("cpt-100", "worker-1", model.StateQueued, pub)

	if fsm.CurrentState() != model.StateQueued {
		t.Fatalf("expected state QUEUED, got %s", fsm.CurrentState())
	}

	// QUEUED -> RUNNING
	if err := fsm.Transition(ctx, model.StateRunning, 0, "Started task"); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// RUNNING -> CHECKPOINT
	if err := fsm.Transition(ctx, model.StateCheckpoint, 25, "Checkpoint 25%"); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// CHECKPOINT -> RUNNING
	if err := fsm.Transition(ctx, model.StateRunning, 25, "Resumed"); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// RUNNING -> COMPLETED
	if err := fsm.Transition(ctx, model.StateCompleted, 100, "Done"); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// COMPLETED -> ARCHIVED
	if err := fsm.Transition(ctx, model.StateArchived, 100, "Archived Dead Drop"); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// ARCHIVED -> PURGED
	if err := fsm.Transition(ctx, model.StatePurged, 100, "Purged scratchpad"); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// Invalid transition from terminal state PURGED -> RUNNING
	if err := fsm.Transition(ctx, model.StateRunning, 0, "Restart"); err == nil {
		t.Fatalf("expected error transitioning from PURGED to RUNNING, got nil")
	}

	if len(pub.events) != 6 {
		t.Fatalf("expected 6 events published, got %d", len(pub.events))
	}
}

func TestFSMFailureTransition(t *testing.T) {
	ctx := context.Background()
	pub := &mockPublisher{}
	fsm := NewFSM("cpt-200", "worker-1", model.StateRunning, pub)

	// RUNNING -> FAILED
	if err := fsm.Transition(ctx, model.StateFailed, 40, "Calculation error"); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	// FAILED -> PURGED
	if err := fsm.Transition(ctx, model.StatePurged, 40, "Cleanup"); err != nil {
		t.Fatalf("unexpected transition error: %v", err)
	}

	if fsm.CurrentState() != model.StatePurged {
		t.Fatalf("expected PURGED, got %s", fsm.CurrentState())
	}
}
