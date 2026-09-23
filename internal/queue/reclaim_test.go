package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"coldharbour/internal/journal"
	"coldharbour/internal/redact"
	"coldharbour/internal/runner"
	"coldharbour/internal/seal"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

const testTenantID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

type countingRunner struct {
	calls int
}

func (c *countingRunner) JobType() string { return "redact" }

func (c *countingRunner) Run(ctx context.Context, input map[string]any, cp *runner.CheckpointRecorder) (map[string]any, error) {
	c.calls++
	return redact.Runner{}.Run(ctx, input, cp)
}

func assertKeyDestroyed(t *testing.T, keys seal.KeyStore, jobID string) {
	t.Helper()
	_, ok, err := keys.Load(context.Background(), journal.DurableIDFor(jobID))
	if ok || err == nil || err.Error() != "seal: job key material is nil" {
		t.Fatalf("Load(%s) ok=%v err=%v, want key destroyed", jobID, ok, err)
	}
}

func assertOneReceipt(t *testing.T, receipts seal.ReceiptStore, jobID string) {
	t.Helper()
	_, ok, err := receipts.Get(context.Background(), journal.DurableIDFor(jobID))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("receipt for %s missing", jobID)
	}
}

func reclaimKeepsRunning(t *testing.T, mr *miniredis.Miniredis, t0 time.Time, idle time.Duration, stream *jobStream, cfg WorkerConfig, deps Deps, ready func() bool) {
	t.Helper()
	mr.SetTime(t0.Add(idle))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunGroup(ctx, stream, cfg, deps)
	}()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case err := <-errCh:
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				if !ready() {
					t.Fatal("worker stopped before reclaim finished")
				}
				return
			}
			t.Fatalf("reclaim RunGroup err = %v, want the worker to keep running", err)
		case <-ticker.C:
			if ready() {
				cancel()
				err := <-errCh
				if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("reclaim RunGroup err = %v, want the worker to keep running", err)
				}
				return
			}
		case <-deadline:
			t.Fatal("reclaim timed out")
		}
	}
}

func TestCrashAfterKeyDestroyFinishes(t *testing.T) {
	mr, stream := startStream(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mr.SetTime(t0)
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	const jobID = "crash-key"
	keys := seal.NewMemoryKeyStore()
	if err := stream.Add(ctx, keys, Job{ID: jobID, Input: redact.M1Fixture, TenantID: testTenantID}); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	priv, receipts := testSigning(t)
	cfg := testWorkerConfig("crash-key")
	err := RunGroup(ctx, stream, cfg, testDeps(store, keys, receipts, priv, func(journal.JobResult) {
		t.Error("emit after key destroy crash")
	}, Hooks{CrashAfterKeyDestroy: func(id string) bool { return id == jobID }}))
	if !errors.Is(err, ErrSimulatedCrash) {
		t.Fatalf("err = %v, want ErrSimulatedCrash", err)
	}
	exists, err := stream.rdb.Exists(ctx, memKey(jobID)).Result()
	if err != nil {
		t.Fatal(err)
	}
	if exists != 1 {
		t.Fatalf("checkpoint hash after crash exists=%d, want 1", exists)
	}

	reclaimKeepsRunning(t, mr, t0, cfg.Idle, stream, testWorkerConfig("crash-key-2"), testDeps(store, keys, receipts, priv, func(journal.JobResult) {}, Hooks{}), func() bool {
		n, err := stream.Len(ctx)
		if err != nil || n != 0 {
			return false
		}
		left, err := stream.rdb.Exists(ctx, memKey(jobID)).Result()
		if err != nil || left != 0 {
			return false
		}
		_, ok, err := receipts.Get(ctx, journal.DurableIDFor(jobID))
		return err == nil && ok
	})
	assertMainLen(t, stream, 0)
	assertOneReceipt(t, receipts, jobID)
	if left, err := stream.rdb.Exists(ctx, memKey(jobID)).Result(); err != nil || left != 0 {
		t.Fatalf("checkpoint hash after reclaim exists=%d err=%v, want 0", left, err)
	}
}

func TestCrashAfterRecordIsPurgedOnReclaim(t *testing.T) {
	mr, stream := startStream(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mr.SetTime(t0)
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	const jobID = "crash-record-bury"
	keys := seal.NewMemoryKeyStore()
	key, err := keys.Ensure(ctx, journal.DurableIDFor(jobID))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := seal.Seal(key, []byte("not a job"))
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{
			"id":        jobID,
			"input":     sealed,
			"job_type":  "nope",
			"tenant_id": testTenantID,
		},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	priv, receipts := testSigning(t)
	cfg := testWorkerConfig("crash-record")
	err = RunGroup(ctx, stream, cfg, testDeps(store, keys, receipts, priv, func(journal.JobResult) {
		t.Error("emit after record crash")
	}, Hooks{CrashAfterRecord: func(id string) bool { return id == jobID }}))
	if !errors.Is(err, ErrSimulatedCrash) {
		t.Fatalf("err = %v, want ErrSimulatedCrash", err)
	}
	if _, _, err := keys.Load(ctx, journal.DurableIDFor(jobID)); err != nil {
		t.Fatalf("Load before reclaim err = %v, want the key still present", err)
	}

	reclaimKeepsRunning(t, mr, t0, cfg.Idle, stream, testWorkerConfig("crash-record-2"), testDeps(store, keys, receipts, priv, func(journal.JobResult) {}, Hooks{}), func() bool {
		_, ok, err := receipts.Get(ctx, journal.DurableIDFor(jobID))
		if err != nil || !ok {
			return false
		}
		_, _, loadErr := keys.Load(ctx, journal.DurableIDFor(jobID))
		return loadErr != nil && loadErr.Error() == "seal: job key material is nil"
	})
	assertOneReceipt(t, receipts, jobID)
	assertKeyDestroyed(t, keys, jobID)
}

func TestCrashAfterRecordDoesNotRerun(t *testing.T) {
	mr, stream := startStream(t)
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mr.SetTime(t0)
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	const jobID = "crash-record-ok"
	keys := seal.NewMemoryKeyStore()
	if err := stream.Add(ctx, keys, Job{ID: jobID, Input: redact.M1Fixture, TenantID: testTenantID}); err != nil {
		t.Fatal(err)
	}
	counter := &countingRunner{}
	store := journal.NewMemoryJournal()
	priv, receipts := testSigning(t)
	deps := Deps{
		Registry:   runner.NewRegistry(counter),
		Journal:    store,
		Keys:       keys,
		Receipts:   receipts,
		SigningKey: priv,
		Emit:       func(journal.JobResult) { t.Error("emit after record crash") },
		Hooks:      Hooks{CrashAfterRecord: func(id string) bool { return id == jobID }},
	}
	cfg := testWorkerConfig("crash-ok")
	err := RunGroup(ctx, stream, cfg, deps)
	if !errors.Is(err, ErrSimulatedCrash) {
		t.Fatalf("err = %v, want ErrSimulatedCrash", err)
	}
	if counter.calls != 1 {
		t.Fatalf("runner calls after crash = %d, want 1", counter.calls)
	}

	deps.Hooks = Hooks{}
	deps.Emit = func(journal.JobResult) {}
	reclaimKeepsRunning(t, mr, t0, cfg.Idle, stream, testWorkerConfig("crash-ok-2"), deps, func() bool {
		if counter.calls > 1 {
			t.Fatalf("runner calls during reclaim = %d, want 1", counter.calls)
		}
		n, err := stream.Len(ctx)
		return err == nil && n == 0 && counter.calls == 1
	})
	if counter.calls != 1 {
		t.Fatalf("runner calls after reclaim = %d, want 1", counter.calls)
	}
}

func TestMissingTenantGoesToDLQAndWorkerContinues(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{"id": "no-tenant", "input": "Email a@b.com x"},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	keys := seal.NewMemoryKeyStore()
	if err := stream.Add(ctx, keys, Job{ID: "ok-tenant", Input: redact.M1Fixture, TenantID: testTenantID}); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	priv, receipts := testSigning(t)
	runCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunGroup(runCtx, stream, testWorkerConfig("no-tenant"), testDeps(store, keys, receipts, priv, func(result journal.JobResult) {
			if result.ID == "ok-tenant" {
				cancel()
			}
		}, Hooks{}))
	}()
	err := <-errCh
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunGroup err = %v, want the worker to stay up", err)
	}
	row, err := store.Load(ctx, "ok-tenant")
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.COMPLETED {
		t.Fatalf("ok-tenant FinalState = %s, want COMPLETED", row.FinalState)
	}
	if _, err := store.Load(ctx, "no-tenant"); !errors.Is(err, journal.ErrUnknownStoredJob) {
		t.Fatalf("no-tenant journal err = %v, want none", err)
	}
	assertDLQLen(t, stream, 1)
	assertStreamField(t, stream, dlqStreamKey, 0, "id", "no-tenant")
	assertNoStreamField(t, stream, dlqStreamKey, 0, "input")
}

func TestUnsealedInputIsBuried(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	const jobID = "plain-1"
	const plain = "Email a@b.com x"
	if err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{"id": jobID, "input": plain, "tenant_id": testTenantID},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	keys := seal.NewMemoryKeyStore()
	store := journal.NewMemoryJournal()
	priv, receipts := testSigning(t)
	if err := consumeOne(t, stream, store, keys, receipts, priv, nil, Hooks{}); err != nil {
		t.Fatal(err)
	}
	row, err := store.Load(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.FAILED {
		t.Fatalf("FinalState = %s, want FAILED", row.FinalState)
	}
	body, err := store.LoadOutput(ctx, jobID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "input key missing") {
		t.Fatalf("output = %s, want input key missing", body)
	}
	assertMainLen(t, stream, 0)
	assertDLQLen(t, stream, 1)
	assertNoStreamField(t, stream, dlqStreamKey, 0, "input")
}
