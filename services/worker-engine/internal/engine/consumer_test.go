package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"coldharbor/worker-engine/internal/config"
	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestConsumerEndToEnd(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := config.DefaultConfig()
	cfg.WorkerID = "worker-e2e-1"
	cfg.BlockDuration = 50 * time.Millisecond
	cfg.HeartbeatInterval = 50 * time.Millisecond
	cfg.RecoveryInterval = 100 * time.Millisecond
	cfg.MinIdleRecoveryTime = 100 * time.Millisecond

	consumer := NewConsumer(cfg, rdb)
	if err := consumer.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("failed to init consumer group: %v", err)
	}

	// Subscribe to events channel to verify pub/sub broadcast
	sub := rdb.Subscribe(ctx, cfg.EventChannel)
	defer sub.Close()
	_, _ = sub.Receive(ctx) // Ensure active subscription

	// Dispatch job to stream
	compartmentID := "cpt-full-e2e"
	jobMsg := model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-test-1",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize": float64(500),
		},
		CreatedAt: time.Now().UTC(),
	}
	jobJSON, _ := json.Marshal(jobMsg)

	msgID, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: cfg.StreamName,
		Values: map[string]any{
			"data": string(jobJSON),
		},
	}).Result()
	if err != nil {
		t.Fatalf("failed to XAdd job: %v", err)
	}

	// Read message via consumer group
	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    cfg.ConsumerGroup,
		Consumer: cfg.WorkerID,
		Streams:  []string{cfg.StreamName, ">"},
		Count:    1,
	}).Result()
	if err != nil || len(streams) == 0 || len(streams[0].Messages) == 0 {
		t.Fatalf("failed to read stream message: %v", err)
	}

	// Process job
	err = consumer.ProcessJob(ctx, streams[0].Messages[0])
	if err != nil {
		t.Fatalf("ProcessJob failed: %v", err)
	}

	// 1. Verify Dead Drop archive sealed in Redis
	archiveKey := consumer.archive.Key(compartmentID)
	archData, err := rdb.Get(ctx, archiveKey).Result()
	if err != nil {
		t.Fatalf("dead drop archive key %s not found: %v", archiveKey, err)
	}

	var dd model.DeadDropPayload
	if err := json.Unmarshal([]byte(archData), &dd); err != nil {
		t.Fatalf("failed to parse dead drop: %v", err)
	}

	if dd.Checksum == "" {
		t.Fatalf("dead drop archive missing checksum")
	}
	valid, err := consumer.archive.VerifyChecksum(&dd)
	if err != nil || !valid {
		t.Fatalf("dead drop checksum verification failed: valid=%v, err=%v", valid, err)
	}
	if dd.Output["reducedSum"] != float64(49201) {
		t.Fatalf("expected reducedSum=49201, got %v", dd.Output["reducedSum"])
	}

	// 2. ZERO-LEAK GUARANTEE: Scratchpad must not exist post-purge
	exists, err := consumer.scratchpad.Exists(ctx, compartmentID)
	if err != nil {
		t.Fatalf("failed to check scratchpad existence: %v", err)
	}
	if exists {
		t.Fatalf("ZERO-LEAK VIOLATION: scratchpad compartment:%s:mem still exists post-purge", compartmentID)
	}

	// 3. XACK: Message must be acknowledged from consumer group PEL
	pendings, err := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: cfg.StreamName,
		Group:  cfg.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err != nil {
		t.Fatalf("failed to check PEL: %v", err)
	}
	for _, p := range pendings {
		if p.ID == msgID {
			t.Fatalf("message %s still present in PEL; expected to be XACKed", msgID)
		}
	}
}

func TestConsumerCrashRecovery(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.WorkerID = "worker-crash-1"

	consumer := NewConsumer(cfg, rdb)
	_ = consumer.InitConsumerGroup(ctx)

	compartmentID := "cpt-crash-recovery"

	// Dispatch job that simulates crash at step 2 (50%)
	jobMsg := model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-crash",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize":            float64(500),
			"simulateCrashAtStep": float64(2),
		},
		CreatedAt: time.Now().UTC(),
	}
	jobJSON, _ := json.Marshal(jobMsg)

	_, _ = rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: cfg.StreamName,
		Values: map[string]any{"data": string(jobJSON)},
	}).Result()

	streams, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    cfg.ConsumerGroup,
		Consumer: cfg.WorkerID,
		Streams:  []string{cfg.StreamName, ">"},
		Count:    1,
	}).Result()
	if err != nil || len(streams) == 0 {
		t.Fatalf("failed to read stream message: %v", err)
	}

	msg := streams[0].Messages[0]

	// First execution triggers simulated crash
	err = consumer.ProcessJob(ctx, msg)
	if err == nil {
		t.Fatalf("expected simulated crash error, got nil")
	}

	// Verify scratchpad holds step 2 (50%) checkpoint
	step, pct, _, err := consumer.scratchpad.GetStep(ctx, compartmentID)
	if err != nil || step != 2 || pct != 50 {
		t.Fatalf("expected scratchpad to hold step 2 checkpoint, got step=%d, pct=%d, err=%v", step, pct, err)
	}

	// Now simulate second worker reclaiming the message and executing without the crash flag
	resumeJobMsg := model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-crash",
		TaskType:      "DATA_REDUCTION",
		Payload: map[string]any{
			"batchSize": float64(500),
		},
		CreatedAt: time.Now().UTC(),
	}
	resumeJSON, _ := json.Marshal(resumeJobMsg)

	msg.Values = map[string]any{"data": string(resumeJSON)}

	cfg2 := config.DefaultConfig()
	cfg2.WorkerID = "worker-recover-2"
	consumer2 := NewConsumer(cfg2, rdb)

	err = consumer2.ProcessJob(ctx, msg)
	if err != nil {
		t.Fatalf("recovery execution failed: %v", err)
	}

	// Verify Dead Drop is now sealed
	dd, err := consumer2.archive.Get(ctx, compartmentID)
	if err != nil || dd == nil {
		t.Fatalf("dead drop archive not found after recovery: %v", err)
	}
	if dd.Output["reducedSum"] != float64(49201) {
		t.Fatalf("unexpected reducedSum: %v", dd.Output["reducedSum"])
	}

	// Verify scratchpad is purged after recovery
	exists, err := consumer2.scratchpad.Exists(ctx, compartmentID)
	if err != nil || exists {
		t.Fatalf("scratchpad must be purged after recovery completion")
	}
}

func TestParseStreamMessageDiscreteFields(t *testing.T) {
	xmsg := redis.XMessage{
		ID: "1600000000000-0",
		Values: map[string]any{
			"compartmentId":  "cpt-discrete-1",
			"context":        "OUTIE",
			"ownerId":        "usr-discrete",
			"taskType":       "CIPHER_STREAM",
			"maxRetries":     "4",
			"timeoutSeconds": "120",
			"createdAt":      "2026-08-18T10:00:00Z",
		},
	}

	job, err := ParseStreamMessage(xmsg)
	if err != nil {
		t.Fatalf("failed to parse discrete stream message: %v", err)
	}

	if job.CompartmentID != "cpt-discrete-1" || job.Context != "OUTIE" || job.MaxRetries != 4 || job.TimeoutSeconds != 120 {
		t.Fatalf("mismatched parsed job: %+v", job)
	}
}

func TestConsumerTransientRetryDoesNotEmitStateFailed(t *testing.T) {
	s := miniredis.RunT(t)
	defer s.Close()

	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	defer rdb.Close()

	ctx := context.Background()
	cfg := config.DefaultConfig()
	cfg.WorkerID = "worker-retry-test"
	cfg.MaxRetries = 2

	consumer := NewConsumer(cfg, rdb)
	mockPub := &mockPublisher{}
	consumer.publisher = mockPub
	consumer.dlq.publisher = mockPub

	if err := consumer.InitConsumerGroup(ctx); err != nil {
		t.Fatalf("failed to init group: %v", err)
	}

	compartmentID := "cpt-transient-retry"
	jobMsg := model.JobMessage{
		CompartmentID: compartmentID,
		Context:       "INNIE",
		OwnerID:       "usr-retry",
		TaskType:      "DATA_REDUCTION",
		MaxRetries:    2,
		Payload: map[string]any{
			"batchSize":       float64(500),
			"simulateFailure": true,
		},
		CreatedAt: time.Now().UTC(),
	}
	jobJSON, _ := json.Marshal(jobMsg)

	_, err := rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: cfg.StreamName,
		Values: map[string]any{"data": string(jobJSON)},
	}).Result()
	if err != nil {
		t.Fatalf("failed to XAdd: %v", err)
	}

	readMsgs, err := rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    cfg.ConsumerGroup,
		Consumer: cfg.WorkerID,
		Streams:  []string{cfg.StreamName, ">"},
		Count:    1,
	}).Result()
	if err != nil || len(readMsgs) == 0 {
		t.Fatalf("failed to read message via XReadGroup: %v", err)
	}
	xmsg := readMsgs[0].Messages[0]

	// 1. First attempt: failure should be transient (newRetries = 1 < maxRetries = 2)
	err = consumer.ProcessJob(ctx, xmsg)
	if err == nil {
		t.Fatalf("expected error on attempt 1")
	}

	// Retry count must be 1
	rc, _ := consumer.dlq.GetRetryCount(ctx, compartmentID)
	if rc != 1 {
		t.Fatalf("expected retry count 1, got %d", rc)
	}

	// Verify events: only QUEUED -> RUNNING emitted, StateFailed must NOT be emitted
	for _, ev := range mockPub.events {
		if ev.ToState == model.StateFailed {
			t.Fatalf("FAIL: StateFailed was prematurely emitted on transient retry attempt!")
		}
	}

	// Message should still be in PEL
	pendings, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: cfg.StreamName,
		Group:  cfg.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if len(pendings) == 0 {
		t.Fatalf("message should remain pending in PEL during retries")
	}

	// Reset recorded events before attempt 2
	mockPub.events = nil

	// 2. Second attempt: reaches maxRetries (newRetries = 2 >= maxRetries = 2) -> routed to DLQ!
	err = consumer.ProcessJob(ctx, xmsg)
	if err != nil {
		t.Fatalf("ProcessJob on final DLQ routing should return nil, got: %v", err)
	}

	// Collect DLQ events: should have StateFailed followed by StatePurged
	hasFailed := false
	hasPurged := false
	for _, ev := range mockPub.events {
		if ev.ToState == model.StateFailed {
			hasFailed = true
		}
		if ev.ToState == model.StatePurged {
			hasPurged = true
		}
	}
	if !hasFailed || !hasPurged {
		t.Fatalf("expected StateFailed and StatePurged on DLQ routing, got: %+v", mockPub.events)
	}

	// Scratchpad must not exist (zero leak)
	exists, err := consumer.scratchpad.Exists(ctx, compartmentID)
	if err != nil || exists {
		t.Fatalf("scratchpad exists after DLQ routing")
	}

	// Message must be acknowledged and removed from PEL
	pendingsAfter, _ := rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: cfg.StreamName,
		Group:  cfg.ConsumerGroup,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if len(pendingsAfter) != 0 {
		t.Fatalf("message should be removed from PEL after DLQ routing")
	}

	// Check message arrived in DLQ stream
	dlqMsgs, err := rdb.XRange(ctx, "coldharbor:jobs:dlq", "-", "+").Result()
	if err != nil || len(dlqMsgs) == 0 {
		t.Fatalf("DLQ stream has no messages: %v", err)
	}
}
