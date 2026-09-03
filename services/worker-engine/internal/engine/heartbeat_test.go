package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestHeartbeatEmitter(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	workerID := "worker-test-hb"
	emitter := NewHeartbeatEmitter(rdb, workerID, 50*time.Millisecond, 30*time.Second)

	emitter.Start(ctx)
	defer emitter.Stop()

	// 1. Verify initial heartbeat emitted immediately
	key := emitter.Key()
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get initial heartbeat: %v", err)
	}

	var hb model.HeartbeatPayload
	if err := json.Unmarshal([]byte(val), &hb); err != nil {
		t.Fatalf("failed to parse heartbeat: %v", err)
	}
	if hb.WorkerID != workerID || hb.Status != "IDLE" {
		t.Fatalf("initial heartbeat mismatch: %+v", hb)
	}

	// 2. Update status to BUSY
	emitter.SetStatus("BUSY", "cpt-999")

	val, err = rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("failed to get updated heartbeat: %v", err)
	}
	if err := json.Unmarshal([]byte(val), &hb); err != nil {
		t.Fatalf("failed to parse updated heartbeat: %v", err)
	}
	if hb.Status != "BUSY" || hb.ActiveCompartmentID != "cpt-999" {
		t.Fatalf("updated heartbeat mismatch: %+v", hb)
	}

	// 3. Verify positive TTL
	ttl := s.TTL(key)
	if ttl <= 0 {
		t.Fatalf("expected positive TTL on heartbeat key, got %v", ttl)
	}
}
