package engine

import (
	"context"
	"testing"

	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestDLQHandler(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	pub := &mockPublisher{}
	scratchpad := NewScratchpadManager(rdb)
	dlq := NewDLQHandler(rdb, "coldharbor:jobs:dlq", scratchpad, pub)

	compartmentID := "cpt-dlq-test"

	// 1. Test retry tracking
	c, err := dlq.GetRetryCount(ctx, compartmentID)
	if err != nil || c != 0 {
		t.Fatalf("expected 0 retries, got %d, err: %v", c, err)
	}

	c1, err := dlq.IncrementRetryCount(ctx, compartmentID)
	if err != nil || c1 != 1 {
		t.Fatalf("expected 1, got %d, err: %v", c1, err)
	}
	c2, err := dlq.IncrementRetryCount(ctx, compartmentID)
	if err != nil || c2 != 2 {
		t.Fatalf("expected 2, got %d, err: %v", c2, err)
	}

	// 2. Populate scratchpad
	err = scratchpad.Write(ctx, compartmentID, map[string]any{"inter": "temp-data"})
	if err != nil {
		t.Fatalf("failed to write scratchpad: %v", err)
	}

	// 3. Create stream entry in coldharbor:jobs and consumer group
	stream := "coldharbor:jobs"
	group := "worker-group"
	msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]any{"data": "test"},
	}).Result()
	if err != nil {
		t.Fatalf("failed to XAdd test message: %v", err)
	}
	_ = rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()

	// Read message to put into PEL
	_, err = rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: "worker-1",
		Streams:  []string{stream, ">"},
		Count:    1,
	}).Result()
	if err != nil {
		t.Fatalf("failed to read message into PEL: %v", err)
	}

	job := &model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-1",
		TaskType:      "DATA_REDUCTION",
	}

	// 4. Route to DLQ
	err = dlq.RouteToDLQ(ctx, stream, group, msgID, "worker-1", job, 3, "unrecoverable corruption")
	if err != nil {
		t.Fatalf("failed to route to DLQ: %v", err)
	}

	// Verify message landed in DLQ stream
	dlqMsgs, err := rdb.XRange(ctx, "coldharbor:jobs:dlq", "-", "+").Result()
	if err != nil || len(dlqMsgs) != 1 {
		t.Fatalf("expected 1 DLQ message, got %d, err: %v", len(dlqMsgs), err)
	}
	if dlqMsgs[0].Values["compartmentId"] != compartmentID {
		t.Fatalf("expected DLQ message compartmentId %s, got %v", compartmentID, dlqMsgs[0].Values["compartmentId"])
	}

	// Verify scratchpad is purged (ZERO LEAK GUARANTEE)
	exists, err := scratchpad.Exists(ctx, compartmentID)
	if err != nil || exists {
		t.Fatalf("scratchpad must be purged after DLQ routing, exists=%v", exists)
	}

	// Verify FAILED and PURGED events were published
	if len(pub.events) != 2 {
		t.Fatalf("expected 2 events (FAILED and PURGED), got %d", len(pub.events))
	}
	if pub.events[0].ToState != model.StateFailed || pub.events[1].ToState != model.StatePurged {
		t.Fatalf("unexpected event states: %+v, %+v", pub.events[0], pub.events[1])
	}
}
