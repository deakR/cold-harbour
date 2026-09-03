package engine

import (
	"context"
	"testing"
	"time"

	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestCheckpointRecording(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	pub := &mockPublisher{}
	compartmentID := "cpt-chk-test"

	fsm := NewFSM(compartmentID, "worker-test", model.StateRunning, pub)
	scratchpad := NewScratchpadManager(rdb)
	recorder := NewCheckpointRecorder(scratchpad, fsm)

	// Step 1: 25%
	err := recorder.Record(ctx, 1, 25, map[string]int{"sum": 25})
	if err != nil {
		t.Fatalf("failed to record 25%% checkpoint: %v", err)
	}

	step, pct, inter, err := scratchpad.GetStep(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to get step: %v", err)
	}
	if step != 1 || pct != 25 || inter != `{"sum":25}` {
		t.Fatalf("scratchpad mismatch at 25%%: step=%d, pct=%d, inter=%s", step, pct, inter)
	}

	// Step 2: 50%
	err = recorder.Record(ctx, 2, 50, map[string]int{"sum": 50})
	if err != nil {
		t.Fatalf("failed to record 50%% checkpoint: %v", err)
	}

	step, pct, inter, err = scratchpad.GetStep(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to get step: %v", err)
	}
	if step != 2 || pct != 50 || inter != `{"sum":50}` {
		t.Fatalf("scratchpad mismatch at 50%%: step=%d, pct=%d, inter=%s", step, pct, inter)
	}

	// Step 3: 75%
	err = recorder.Record(ctx, 3, 75, map[string]int{"sum": 75})
	if err != nil {
		t.Fatalf("failed to record 75%% checkpoint: %v", err)
	}

	step, pct, inter, err = scratchpad.GetStep(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to get step: %v", err)
	}
	if step != 3 || pct != 75 || inter != `{"sum":75}` {
		t.Fatalf("scratchpad mismatch at 75%%: step=%d, pct=%d, inter=%s", step, pct, inter)
	}

	// State should be back to RUNNING after checkpoint
	if fsm.CurrentState() != model.StateRunning {
		t.Fatalf("expected FSM to be in RUNNING after checkpoint, got %s", fsm.CurrentState())
	}

	// Each Record call triggers 2 transitions: RUNNING -> CHECKPOINT, CHECKPOINT -> RUNNING
	if len(pub.events) != 6 {
		t.Fatalf("expected 6 events emitted for 3 checkpoints, got %d", len(pub.events))
	}
}

func TestRecoveryManagerInspection(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	scratchpad := NewScratchpadManager(rdb)
	recovery := NewRecoveryManager(rdb, "coldharbor:jobs", "worker-group", "worker-rec", 100*time.Millisecond, scratchpad)

	compartmentID := "cpt-crashed-job"

	// Simulate intermediate checkpoint left by crashed worker
	if err := scratchpad.SetStep(ctx, compartmentID, 2, 50, `{"processed":250}`); err != nil {
		t.Fatalf("failed to set intermediate step: %v", err)
	}

	// Inspect via recovery manager
	step, pct, intermediate, err := recovery.InspectCheckpoint(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to inspect checkpoint: %v", err)
	}
	if step != 2 || pct != 50 || intermediate != `{"processed":250}` {
		t.Fatalf("mismatched recovered checkpoint: step=%d, pct=%d, intermediate=%s", step, pct, intermediate)
	}
}
