package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"coldharbor/worker-engine/internal/config"
	"coldharbor/worker-engine/internal/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type trackingExecutor struct {
	delay      time.Duration
	waitForCtx bool
	calls      atomic.Int64
	active     atomic.Int64
	maxActive  atomic.Int64
}

func (e *trackingExecutor) Execute(ctx context.Context, _ *model.JobMessage, recorder *CheckpointRecorder) (*TaskResult, error) {
	e.calls.Add(1)
	active := e.active.Add(1)
	defer e.active.Add(-1)
	for {
		max := e.maxActive.Load()
		if active <= max || e.maxActive.CompareAndSwap(max, active) {
			break
		}
	}

	if err := recorder.Record(ctx, 1, 25, map[string]any{"started": true}); err != nil {
		return nil, err
	}
	if e.waitForCtx {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	select {
	case <-time.After(e.delay):
		return &TaskResult{Output: map[string]any{"status": "SUCCESS"}, DurationMs: 1}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func testXMessage(t *testing.T, id, compartmentID string, mutate func(*model.JobMessage)) redis.XMessage {
	t.Helper()
	job := &model.JobMessage{
		CompartmentID: compartmentID,
		TaskType:      "DATA_REDUCTION",
		Payload:       map[string]any{"batchSize": int64(10)},
	}
	if mutate != nil {
		mutate(job)
	}
	data, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	return redis.XMessage{ID: id, Values: map[string]any{"data": string(data)}}
}

func TestProcessJobTimeoutUsesRetryAndDLQCleanup(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := config.DefaultConfig()
	cfg.MaxRetries = 1
	consumer := NewConsumer(cfg, rdb)
	executor := &trackingExecutor{waitForCtx: true}
	consumer.runner = executor

	msg := testXMessage(t, "1-0", "timeout-job", func(job *model.JobMessage) {
		job.TimeoutSeconds = 1
	})
	start := time.Now()
	if err := consumer.ProcessJob(context.Background(), msg); err != nil {
		t.Fatalf("terminal timeout should route to DLQ: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed > 3*time.Second {
		t.Fatalf("timeout elapsed outside expected range: %v", elapsed)
	}
	if consumer.metrics.TimedOut.Load() != 1 {
		t.Fatalf("expected one timeout metric")
	}
	exists, err := consumer.scratchpad.Exists(context.Background(), "timeout-job")
	if err != nil || exists {
		t.Fatalf("scratchpad was not cleaned after timed-out terminal failure: exists=%v err=%v", exists, err)
	}
	dlq, err := rdb.XRange(context.Background(), cfg.DLQStreamName, "-", "+").Result()
	if err != nil || len(dlq) != 1 || !strings.Contains(dlq[0].Values["reason"].(string), "timed out") {
		t.Fatalf("timeout was not classified in DLQ: messages=%v err=%v", dlq, err)
	}
}

func TestCompletedMessageDoesNotExecuteAgain(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	consumer := NewConsumer(config.DefaultConfig(), rdb)
	executor := &trackingExecutor{}
	consumer.runner = executor
	msg := testXMessage(t, "2-0", "completed-job", nil)

	if err := consumer.ProcessJob(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if err := consumer.ProcessJob(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	if calls := executor.calls.Load(); calls != 1 {
		t.Fatalf("completed delivery executed %d times", calls)
	}
	if consumer.metrics.Deduped.Load() != 1 {
		t.Fatalf("expected replay to increment deduplication metric")
	}
	if n, err := rdb.XLen(context.Background(), consumer.cfg.AuditStreamName).Result(); err != nil || n != 1 {
		t.Fatalf("expected one durable audit envelope, got %d: %v", n, err)
	}
	if n, err := rdb.XLen(context.Background(), consumer.cfg.EventStreamName).Result(); err != nil || n == 0 {
		t.Fatalf("expected durable lifecycle events, got %d: %v", n, err)
	}
	exists, err := consumer.scratchpad.Exists(context.Background(), "completed-job")
	if err != nil || exists {
		t.Fatalf("replay recreated scratchpad: exists=%v err=%v", exists, err)
	}
}

func TestSimulatedCrashDoesNotConsumeRetry(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	consumer := NewConsumer(config.DefaultConfig(), rdb)
	msg := testXMessage(t, "3-0", "crash-budget-job", func(job *model.JobMessage) {
		job.Payload["simulateCrashAtStep"] = int64(1)
	})
	if err := consumer.ProcessJob(context.Background(), msg); err == nil {
		t.Fatal("expected simulated crash")
	}
	retries, err := consumer.dlq.GetRetryCount(context.Background(), "crash-budget-job")
	if err != nil || retries != 0 {
		t.Fatalf("simulated crash consumed retry budget: retries=%d err=%v", retries, err)
	}
}

func TestWorkerPoolBoundsConcurrentExecution(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	cfg := config.DefaultConfig()
	cfg.Concurrency = 3
	cfg.BlockDuration = 20 * time.Millisecond
	cfg.RecoveryInterval = time.Hour
	consumer := NewConsumer(cfg, rdb)
	executor := &trackingExecutor{delay: 75 * time.Millisecond}
	consumer.runner = executor

	ctx, cancel := context.WithCancel(context.Background())
	consumer.Start(ctx)
	for i := 0; i < 9; i++ {
		if !consumer.submit(ctx, testXMessage(t, fmt.Sprintf("%d-0", i+10), fmt.Sprintf("pool-job-%d", i), nil)) {
			t.Fatal("pool stopped accepting jobs")
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for consumer.metrics.Completed.Load() < 9 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	consumer.Stop()

	if calls := executor.calls.Load(); calls != 9 {
		t.Fatalf("expected 9 executions, got %d", calls)
	}
	if max := executor.maxActive.Load(); max != int64(cfg.Concurrency) {
		t.Fatalf("expected max concurrency %d, got %d", cfg.Concurrency, max)
	}
}

func TestParseStreamMessageInt64Fields(t *testing.T) {
	job, err := ParseStreamMessage(redis.XMessage{
		ID: "4-0",
		Values: map[string]any{
			"compartmentId":  "numeric-job",
			"maxRetries":     int64(7),
			"timeoutSeconds": int64(11),
			"payload":        map[string]any{"batchSize": int64(13)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.MaxRetries != 7 || job.TimeoutSeconds != 11 || job.Payload["batchSize"] != int64(13) {
		t.Fatalf("int64 fields were not preserved: %+v", job)
	}
}
