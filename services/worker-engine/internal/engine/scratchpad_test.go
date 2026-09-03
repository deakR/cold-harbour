package engine

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestScratchpadLifecycleAndPurge(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	mgr := NewScratchpadManager(rdb)
	compartmentID := "cpt-leak-test"

	// 1. Invariant before use: Exists == false
	exists, err := mgr.Exists(ctx, compartmentID)
	if err != nil {
		t.Fatalf("unexpected exists error: %v", err)
	}
	if exists {
		t.Fatalf("expected scratchpad not to exist before writing")
	}

	// 2. Write initial calculation data
	err = mgr.Write(ctx, compartmentID, map[string]any{
		"counter": 42,
		"buffer":  "chunk_abc",
	})
	if err != nil {
		t.Fatalf("failed to write scratchpad: %v", err)
	}

	exists, err = mgr.Exists(ctx, compartmentID)
	if err != nil || !exists {
		t.Fatalf("expected scratchpad to exist after writing")
	}

	// 3. Set and retrieve step
	err = mgr.SetStep(ctx, compartmentID, 2, 50, map[string]int{"partialSum": 100})
	if err != nil {
		t.Fatalf("failed to set step: %v", err)
	}

	step, pct, intermediate, err := mgr.GetStep(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to get step: %v", err)
	}
	if step != 2 || pct != 50 || intermediate != `{"partialSum":100}` {
		t.Fatalf("unexpected step data: step=%d, pct=%d, intermediate=%s", step, pct, intermediate)
	}

	// 4. Atomic purge via DEL
	err = mgr.Purge(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to purge scratchpad: %v", err)
	}

	// 5. Mandatory zero-leak guarantee verification: EXISTS == 0
	exists, err = mgr.Exists(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to check exists post-purge: %v", err)
	}
	if exists {
		t.Fatalf("ZERO-LEAK VIOLATION: compartment:%s:mem still exists post-purge", compartmentID)
	}
}
