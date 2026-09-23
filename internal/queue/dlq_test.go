package queue

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
	"time"

	"coldharbour/internal/checkpoint"
	"coldharbour/internal/journal"
	"coldharbour/internal/redact"
	"coldharbour/internal/seal"

	"github.com/redis/go-redis/v9"
)

func job5Checksum(t *testing.T) string {
	t.Helper()
	want, err := redact.RedactPII(redact.M1Fixture)
	if err != nil {
		t.Fatal(err)
	}
	return journal.ChecksumOf(redact.MapResult(want))
}

func TestFailJobThreeAttemptsThenBury(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "job-5", Input: redact.M1Fixture}
	if err := addFailJob(ctx, stream, job); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	keys := seal.NewMemoryKeyStore()
	priv, receipts := testSigning(t)
	wantChecksum := job5Checksum(t)

	if err := consumeOne(t, stream, store, keys, receipts, priv, nil, failThese(job.ID)); err != nil {
		t.Fatal(err)
	}
	assertRetry(t, stream, job.ID, "1")
	assertDLQLen(t, stream, 0)
	assertPending(t, stream, 0)
	assertMainLen(t, stream, 1)
	if _, err := store.Load(ctx, job.ID); !errors.Is(err, journal.ErrUnknownStoredJob) {
		t.Fatalf("Load after attempt 1 err = %v, want ErrUnknownStoredJob", err)
	}
	if exists, err := stream.rdb.Exists(ctx, memKey(job.ID)).Result(); err != nil {
		t.Fatal(err)
	} else if exists != 1 {
		t.Fatalf("mem after attempt 1 exists=%d, want 1", exists)
	}

	if err := consumeOne(t, stream, store, keys, receipts, priv, nil, failThese(job.ID)); err != nil {
		t.Fatal(err)
	}
	assertRetry(t, stream, job.ID, "2")
	assertDLQLen(t, stream, 0)
	assertPending(t, stream, 0)
	assertMainLen(t, stream, 1)
	if _, err := store.Load(ctx, job.ID); !errors.Is(err, journal.ErrUnknownStoredJob) {
		t.Fatalf("Load after attempt 2 err = %v, want ErrUnknownStoredJob", err)
	}
	if exists, err := stream.rdb.Exists(ctx, memKey(job.ID)).Result(); err != nil {
		t.Fatal(err)
	} else if exists != 1 {
		t.Fatalf("mem after attempt 2 exists=%d, want 1", exists)
	}

	if err := consumeOne(t, stream, store, keys, receipts, priv, nil, failThese(job.ID)); err != nil {
		t.Fatal(err)
	}
	assertRetryGone(t, stream, job.ID)
	assertDLQLen(t, stream, 1)
	assertPending(t, stream, 0)
	assertMainLen(t, stream, 0)
	assertStreamField(t, stream, dlqStreamKey, 0, "id", job.ID)
	assertNoStreamField(t, stream, dlqStreamKey, 0, "input")
	row, err := store.Load(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.FAILED {
		t.Fatalf("FinalState = %s, want FAILED", row.FinalState)
	}
	if row.Checksum != wantChecksum {
		t.Fatalf("Checksum = %s, want %s", row.Checksum, wantChecksum)
	}
	if exists, err := stream.rdb.Exists(ctx, memKey(job.ID)).Result(); err != nil {
		t.Fatal(err)
	} else if exists != 0 {
		t.Fatalf("mem after bury exists=%d, want 0", exists)
	}
	if _, ok, err := receipts.Get(ctx, journal.DurableIDFor(job.ID)); err != nil {
		t.Fatal(err)
	} else if !ok {
		t.Fatal("missing purge receipt after bury")
	}

}

func TestRunGroupBuriesWithoutEmit(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := addFailJob(ctx, stream, Job{ID: "job-5", Input: redact.M1Fixture}); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	emitted := false
	runGroupUntil(t, stream, store, func(journal.JobResult) {
		emitted = true
	}, func() bool {
		n, err := stream.rdb.XLen(ctx, dlqStreamKey).Result()
		if err != nil {
			return false
		}
		pending, err := stream.rdb.XPending(ctx, jobsStreamKey, workerGroup).Result()
		if err != nil {
			return false
		}
		return n == 1 && pending.Count == 0
	}, failThese("job-5"))
	if emitted {
		t.Fatal("emit ran on failure")
	}
	row, err := store.Load(ctx, "job-5")
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.FAILED {
		t.Fatalf("FinalState = %s, want FAILED", row.FinalState)
	}
	if row.Checksum != job5Checksum(t) {
		t.Fatalf("Checksum = %s, want %s", row.Checksum, job5Checksum(t))
	}
}

func TestReplayAtThreeBuriesAgain(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.rdb.Set(ctx, retryKey("job-5"), 3, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if err := addFailJob(ctx, stream, Job{ID: "job-5", Input: redact.M1Fixture}); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	priv, receipts := testSigning(t)
	if err := consumeOne(t, stream, store, seal.NewMemoryKeyStore(), receipts, priv, nil, failThese("job-5")); err != nil {
		t.Fatal(err)
	}
	assertMainLen(t, stream, 0)
	assertDLQLen(t, stream, 1)
	assertPending(t, stream, 0)
	assertRetryGone(t, stream, "job-5")
	row, err := store.Load(ctx, "job-5")
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.FAILED {
		t.Fatalf("FinalState = %s, want FAILED", row.FinalState)
	}
}

func TestCrashStillWinsOverFailure(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.Add(ctx, Job{ID: "crash-fail", Input: redact.M1Fixture}); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	priv, receipts := testSigning(t)
	err := RunGroup(ctx, stream, testWorkerConfig("crash-fail"), testDeps(store, seal.NewMemoryKeyStore(), receipts, priv, func(journal.JobResult) {
		t.Error("emit after crash")
	}, Hooks{
		CrashAfterStep1: func(id string) bool { return id == "crash-fail" },
		Fail:            func(id string) bool { return id == "crash-fail" },
	}))
	if !errors.Is(err, checkpoint.ErrSimulatedCrash) {
		t.Fatalf("err = %v, want ErrSimulatedCrash", err)
	}
	if _, err := store.Load(ctx, "crash-fail"); !errors.Is(err, journal.ErrUnknownStoredJob) {
		t.Fatalf("journal after crash = %v, want ErrUnknownStoredJob", err)
	}
	assertDLQLen(t, stream, 0)
	assertPending(t, stream, 1)
}

func TestPoisonJobIsBuriedAndWorkerContinues(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{
			"id":       "poison-1",
			"input":    "x",
			"job_type": "nope",
		},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Add(ctx, Job{ID: "ok-1", Input: redact.M1Fixture}); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	keys := seal.NewMemoryKeyStore()
	priv, receipts := testSigning(t)
	runCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	emitted := make(chan string, 1)
	go func() {
		done <- RunGroup(runCtx, stream, testWorkerConfig("poison"), testDeps(store, keys, receipts, priv, func(result journal.JobResult) {
			emitted <- result.ID
		}, Hooks{}))
	}()
	select {
	case id := <-emitted:
		if id != "ok-1" {
			t.Fatalf("emitted %q, want ok-1", id)
		}
	case err := <-done:
		t.Fatalf("RunGroup returned before ok-1: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ok-1")
	}
	cancel()
	<-done

	row, err := store.Load(ctx, "poison-1")
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.FAILED {
		t.Fatalf("poison FinalState = %s, want FAILED", row.FinalState)
	}
	ok, err := store.Load(ctx, "ok-1")
	if err != nil {
		t.Fatal(err)
	}
	if ok.FinalState != journal.COMPLETED {
		t.Fatalf("ok FinalState = %s, want COMPLETED", ok.FinalState)
	}
	assertDLQLen(t, stream, 1)
	assertStreamField(t, stream, dlqStreamKey, 0, "id", "poison-1")
	assertNoStreamField(t, stream, dlqStreamKey, 0, "input")
	assertMainLen(t, stream, 0)
	if keys.Has(journal.DurableIDFor("poison-1")) {
		t.Fatal("poison job created a key")
	}
}

func TestCompletedJobLeavesNoStreamEntry(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.Add(ctx, Job{ID: "job-ok", Input: redact.M1Fixture}); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	priv, receipts := testSigning(t)
	if err := consumeOne(t, stream, store, seal.NewMemoryKeyStore(), receipts, priv, nil, Hooks{}); err != nil {
		t.Fatal(err)
	}
	assertMainLen(t, stream, 0)
	assertPending(t, stream, 0)
	assertDLQLen(t, stream, 0)
	row, err := store.Load(ctx, "job-ok")
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.COMPLETED {
		t.Fatalf("FinalState = %s, want COMPLETED", row.FinalState)
	}
}

func failThese(ids ...string) Hooks {
	want := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	return Hooks{Fail: func(jobID string) bool {
		_, ok := want[jobID]
		return ok
	}}
}

func addFailJob(ctx context.Context, stream *jobStream, job Job) error {
	return stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{
			"id":    job.ID,
			"input": job.Input,
		},
	}).Err()
}

func consumeOne(t *testing.T, stream *jobStream, store journal.Journal, keys seal.KeyStore, receipts seal.ReceiptStore, priv ed25519.PrivateKey, emit func(journal.JobResult), hooks Hooks) error {
	t.Helper()
	if emit == nil {
		emit = func(journal.JobResult) {}
	}
	ctx := context.Background()
	hash := &memHash{rdb: stream.rdb, keys: keys}
	purge := &purger{rdb: stream.rdb, keys: keys, receipts: receipts, priv: priv}
	streams, err := stream.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    workerGroup,
		Consumer: "dlq-test",
		Streams:  []string{jobsStreamKey, ">"},
		Count:    1,
		Block:    200 * time.Millisecond,
	}).Result()
	if err != nil {
		return err
	}
	if len(streams) == 0 || len(streams[0].Messages) == 0 {
		return errors.New("no message")
	}
	return dispatch(ctx, stream, hash, purge, testRegistry(), streams[0].Messages[0], store, emit, hooks)
}

func runGroupUntil(t *testing.T, stream *jobStream, store journal.Journal, emit func(journal.JobResult), pred func() bool, hooks Hooks) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	priv, receipts := testSigning(t)
	go func() {
		done <- RunGroup(ctx, stream, testWorkerConfig("dlq"), testDeps(store, seal.NewMemoryKeyStore(), receipts, priv, emit, hooks))
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pred() {
			cancel()
			<-done
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	err := <-done
	t.Fatalf("timeout waiting for bury, runGroup err = %v", err)
}

func assertRetry(t *testing.T, stream *jobStream, jobID, want string) {
	t.Helper()
	got, err := stream.rdb.Get(context.Background(), retryKey(jobID)).Result()
	if err != nil {
		t.Fatalf("GET %s err = %v, want %s", retryKey(jobID), err, want)
	}
	if got != want {
		t.Fatalf("GET %s = %q, want %q", retryKey(jobID), got, want)
	}
}

func assertRetryGone(t *testing.T, stream *jobStream, jobID string) {
	t.Helper()
	err := stream.rdb.Get(context.Background(), retryKey(jobID)).Err()
	if !errors.Is(err, redis.Nil) {
		t.Fatalf("GET %s err = %v, want redis.Nil", retryKey(jobID), err)
	}
}

func assertDLQLen(t *testing.T, stream *jobStream, want int64) {
	t.Helper()
	n, err := stream.rdb.XLen(context.Background(), dlqStreamKey).Result()
	if err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("XLEN %s = %d, want %d", dlqStreamKey, n, want)
	}
}

func assertMainLen(t *testing.T, stream *jobStream, want int64) {
	t.Helper()
	n, err := stream.Len(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("XLEN %s = %d, want %d", jobsStreamKey, n, want)
	}
}

func assertPending(t *testing.T, stream *jobStream, want int64) {
	t.Helper()
	pending, err := stream.rdb.XPending(context.Background(), jobsStreamKey, workerGroup).Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != want {
		t.Fatalf("pending = %d, want %d", pending.Count, want)
	}
}

func assertNoStreamField(t *testing.T, stream *jobStream, key string, index int64, field string) {
	t.Helper()
	entries, err := stream.rdb.XRange(context.Background(), key, "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(entries)) <= index {
		t.Fatalf("XRANGE %s len = %d, want index %d", key, len(entries), index)
	}
	fields := valuesToFields(entries[index].Values)
	if _, ok := fields[field]; ok {
		t.Fatalf("%s[%d] has %s", key, index, field)
	}
}

func assertStreamField(t *testing.T, stream *jobStream, key string, index int64, field, want string) {
	t.Helper()
	entries, err := stream.rdb.XRange(context.Background(), key, "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(entries)) <= index {
		t.Fatalf("XRANGE %s len = %d, want index %d", key, len(entries), index)
	}
	fields := valuesToFields(entries[index].Values)
	if fields[field] != want {
		t.Fatalf("%s[%d].%s = %q, want %q", key, index, field, fields[field], want)
	}
}
