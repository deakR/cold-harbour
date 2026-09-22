package queue

import (
	"context"
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
	return journal.ChecksumOf(want)
}

func TestFailJobThreeAttemptsThenRedrive(t *testing.T) {
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
	wantChecksum := job5Checksum(t)

	if err := consumeOne(t, stream, store, keys, nil); err != nil {
		t.Fatal(err)
	}
	assertRetry(t, stream, job.ID, "1")
	assertDLQLen(t, stream, 0)
	assertPending(t, stream, 0)
	if _, err := store.Load(ctx, job.ID); !errors.Is(err, journal.ErrUnknownStoredJob) {
		t.Fatalf("Load after attempt 1 err = %v, want ErrUnknownStoredJob", err)
	}

	if err := consumeOne(t, stream, store, keys, nil); err != nil {
		t.Fatal(err)
	}
	assertRetry(t, stream, job.ID, "2")
	assertDLQLen(t, stream, 0)
	assertPending(t, stream, 0)
	if _, err := store.Load(ctx, job.ID); !errors.Is(err, journal.ErrUnknownStoredJob) {
		t.Fatalf("Load after attempt 2 err = %v, want ErrUnknownStoredJob", err)
	}

	if err := consumeOne(t, stream, store, keys, nil); err != nil {
		t.Fatal(err)
	}
	assertRetryGone(t, stream, job.ID)
	assertDLQLen(t, stream, 1)
	assertPending(t, stream, 0)
	assertMainLen(t, stream, 3)
	assertStreamField(t, stream, dlqStreamKey, 0, "simulateFailure", "1")
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

	if err := Redrive(ctx, stream); err != nil {
		t.Fatal(err)
	}
	assertDLQLen(t, stream, 1)
	assertRetry(t, stream, job.ID, "0")
	assertStreamField(t, stream, jobsStreamKey, 3, "simulateFailure", "1")

	if err := consumeOne(t, stream, store, keys, nil); err != nil {
		t.Fatal(err)
	}
	if err := Redrive(ctx, stream); !errors.Is(err, errRetryLive) {
		t.Fatalf("redrive while retry is live err = %v, want errRetryLive", err)
	}

	if err := consumeOne(t, stream, store, keys, nil); err != nil {
		t.Fatal(err)
	}
	if err := consumeOne(t, stream, store, keys, nil); err != nil {
		t.Fatal(err)
	}
	assertRetryGone(t, stream, job.ID)
	assertDLQLen(t, stream, 2)
	assertPending(t, stream, 0)
	row, err = store.Load(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.FAILED || row.Checksum != wantChecksum {
		t.Fatalf("second bury row = %+v", row)
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
	})
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
	if err := consumeOne(t, stream, store, seal.NewMemoryKeyStore(), nil); err != nil {
		t.Fatal(err)
	}
	assertMainLen(t, stream, 1)
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
	if err := stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{
			"id":                  "crash-fail",
			"input":               redact.M1Fixture,
			"simulateCrashAtStep": "1",
			"simulateFailure":     "1",
		},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	err := RunGroup(ctx, stream, testWorkerConfig("crash-fail"), store, seal.NewMemoryKeyStore(), func(journal.JobResult) {
		t.Error("emit after crash")
	})
	if !errors.Is(err, checkpoint.ErrSimulatedCrash) {
		t.Fatalf("err = %v, want ErrSimulatedCrash", err)
	}
	if _, err := store.Load(ctx, "crash-fail"); !errors.Is(err, journal.ErrUnknownStoredJob) {
		t.Fatalf("journal after crash = %v, want ErrUnknownStoredJob", err)
	}
	assertDLQLen(t, stream, 0)
	assertPending(t, stream, 1)
}

func TestRedriveEmptyDLQ(t *testing.T) {
	_, stream := startStream(t)
	err := Redrive(context.Background(), stream)
	if !errors.Is(err, errEmptyDLQ) {
		t.Fatalf("err = %v, want errEmptyDLQ", err)
	}
}

func addFailJob(ctx context.Context, stream *jobStream, job Job) error {
	return stream.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{
			"id":              job.ID,
			"input":           job.Input,
			"simulateFailure": "1",
		},
	}).Err()
}

func consumeOne(t *testing.T, stream *jobStream, store journal.Journal, keys seal.KeyStore, emit func(journal.JobResult)) error {
	t.Helper()
	if emit == nil {
		emit = func(journal.JobResult) {}
	}
	ctx := context.Background()
	hash := &memHash{rdb: stream.rdb, keys: keys}
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
	return dispatch(ctx, stream, hash, streams[0].Messages[0], store, emit)
}

func runGroupUntil(t *testing.T, stream *jobStream, store journal.Journal, emit func(journal.JobResult), pred func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunGroup(ctx, stream, testWorkerConfig("dlq"), store, seal.NewMemoryKeyStore(), emit)
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
