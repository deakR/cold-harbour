package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestParseStreamIDRejectsSpecials(t *testing.T) {
	for _, s := range []string{"$", "*", "+", "-", "", "abc", "1", "1-2-3", "1-", "-1"} {
		if _, err := ParseStreamID(s); err == nil {
			t.Errorf("ParseStreamID(%q) err = nil, want error", s)
		}
	}
}

func TestParseStreamIDAcceptsNN(t *testing.T) {
	zero, err := ParseStreamID("0-0")
	if err != nil {
		t.Fatalf("ParseStreamID(%q) err = %v", "0-0", err)
	}
	if zero != StreamIDZero() {
		t.Fatalf("ParseStreamID(%q) = %+v, want zero", "0-0", zero)
	}
	if zero.String() != "0-0" {
		t.Fatalf("String() = %q, want %q", zero.String(), "0-0")
	}

	id, err := ParseStreamID("1700000000000-3")
	if err != nil {
		t.Fatalf("ParseStreamID(%q) err = %v", "1700000000000-3", err)
	}
	if id.String() != "1700000000000-3" {
		t.Fatalf("String() = %q, want %q", id.String(), "1700000000000-3")
	}
}

func TestParseJob(t *testing.T) {
	entry, err := ParseStreamID("99-1")
	if err != nil {
		t.Fatal(err)
	}

	job, err := parseJob(entry, map[string]string{"id": "job-cli", "input": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if job != (Job{ID: "job-cli", Input: "hello"}) {
		t.Fatalf("got %+v, want id field", job)
	}

	job, err = parseJob(entry, map[string]string{"input": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if job != (Job{ID: "99-1", Input: "hello"}) {
		t.Fatalf("got %+v, want entry id 99-1", job)
	}

	job, err = parseJob(entry, map[string]string{"id": "", "input": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if job != (Job{ID: "99-1", Input: "hello"}) {
		t.Fatalf("got %+v, want entry id when id is empty", job)
	}

	if _, err := parseJob(entry, map[string]string{"id": "x"}); !errors.Is(err, errMissingInput) {
		t.Fatalf("missing input err = %v, want errMissingInput", err)
	}
	if _, err := parseJob(entry, map[string]string{"id": "x", "input": ""}); !errors.Is(err, errMissingInput) {
		t.Fatalf("empty input err = %v, want errMissingInput", err)
	}
}

func TestParseClaim(t *testing.T) {
	entry, err := ParseStreamID("99-1")
	if err != nil {
		t.Fatal(err)
	}
	c, err := parseClaim(entry, map[string]string{"id": "crash-1", "input": "hello", "simulateCrashAtStep": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if c.job != (Job{ID: "crash-1", Input: "hello"}) {
		t.Fatalf("job = %+v", c.job)
	}
	if c.crash != CrashAfterStep1 {
		t.Fatalf("crash = %v, want CrashAfterStep1", c.crash)
	}

	c, err = parseClaim(entry, map[string]string{"input": "hello", "simulateCrashAtStep": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if c.crash != CrashNever {
		t.Fatalf("crash = %v, want CrashNever", c.crash)
	}
}

func TestStreamPicksUpSittingJob(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}

	input := "Email alice.nguyen@school.edu about the lab report."
	if err := stream.Add(ctx, Job{ID: "job-cli", Input: input}); err != nil {
		t.Fatal(err)
	}

	want, err := RedactPII(input)
	if err != nil {
		t.Fatal(err)
	}

	got, err := runGroupOnce(t, stream, testWorkerConfig("sit"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "job-cli" {
		t.Fatalf("ID = %q, want %q", got.ID, "job-cli")
	}
	if got.Result != want {
		t.Fatalf("Result = %+v, want %+v", got.Result, want)
	}
	if got.Result.RedactedText != "Email [EMAIL_REDACTED] about the lab report." {
		t.Fatalf("RedactedText = %q, want Email [EMAIL_REDACTED] about the lab report.", got.Result.RedactedText)
	}
}

func TestSeedIfEmpty(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()

	if err := seedIfEmpty(ctx, stream, []Job{{ID: "a", Input: "x"}}); err != nil {
		t.Fatal(err)
	}
	n, err := stream.Len(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("Len after empty seed = %d, want 1", n)
	}

	if err := seedIfEmpty(ctx, stream, []Job{{ID: "b", Input: "y"}}); err != nil {
		t.Fatal(err)
	}
	n, err = stream.Len(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("Len after second seed = %d, want 1", n)
	}
}

func TestRunGroupSkipsBadEntry(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}

	raw := redis.NewClient(&redis.Options{Addr: stream.rdb.Options().Addr})
	t.Cleanup(func() { _ = raw.Close() })
	if err := raw.XAdd(ctx, &redis.XAddArgs{
		Stream: jobsStreamKey,
		Values: map[string]any{"id": "bad"},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := stream.Add(ctx, Job{ID: "good", Input: "Email a@b.com x"}); err != nil {
		t.Fatal(err)
	}

	got, err := runGroupOnce(t, stream, testWorkerConfig("skip"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "good" {
		t.Fatalf("ID = %q, want good", got.ID)
	}

	pending, err := stream.rdb.XPending(ctx, jobsStreamKey, workerGroup).Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 0 {
		t.Fatalf("pending after poison skip = %d, want 0", pending.Count)
	}
}

func TestRunGroupRecoversViaAutoClaim(t *testing.T) {
	mr, stream := startStream(t)
	ctx := context.Background()
	cfg1 := testWorkerConfig("w1")
	cfg2 := testWorkerConfig("w2")

	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mr.SetTime(t0)

	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.enqueue(ctx, Job{ID: "crash-1", Input: m1Fixture}, CrashAfterStep1); err != nil {
		t.Fatal(err)
	}

	err := runGroup(ctx, stream, cfg1, func(JobResult) {})
	if !errors.Is(err, ErrSimulatedCrash) {
		t.Fatalf("worker 1 err = %v, want ErrSimulatedCrash", err)
	}

	pending, err := stream.rdb.XPending(ctx, jobsStreamKey, workerGroup).Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 1 {
		t.Fatalf("pending after crash = %d, want 1", pending.Count)
	}

	mr.SetTime(t0.Add(cfg1.Idle))

	got, err := runGroupOnce(t, stream, cfg2)
	if err != nil {
		t.Fatal(err)
	}
	want, err := RedactPII(m1Fixture)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "crash-1" {
		t.Fatalf("ID = %q, want crash-1", got.ID)
	}
	if got.Result != want {
		t.Fatalf("Result = %+v, want %+v", got.Result, want)
	}

	step, err := stream.rdb.HGet(ctx, memKey("crash-1"), memFieldStep).Result()
	if err != nil {
		t.Fatal(err)
	}
	if step != "2" {
		t.Fatalf("hash step = %q, want 2", step)
	}
	step1, err := stream.rdb.HGet(ctx, memKey("crash-1"), memFieldStep1).Result()
	if err != nil {
		t.Fatal(err)
	}
	if step1 != "1" {
		t.Fatalf("step1 = %q, want 1", step1)
	}

	pending, err = stream.rdb.XPending(ctx, jobsStreamKey, workerGroup).Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 0 {
		t.Fatalf("pending after reclaim = %d, want 0", pending.Count)
	}
}

func TestEnsureGroupBusyGroup(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatalf("second ensureGroup err = %v", err)
	}
}

func TestRedisAddr(t *testing.T) {
	t.Setenv("REDIS_ADDR", "")
	if got := redisAddr(); got != "127.0.0.1:6379" {
		t.Fatalf("default redisAddr() = %q, want 127.0.0.1:6379", got)
	}
	t.Setenv("REDIS_ADDR", "10.0.0.1:6380")
	if got := redisAddr(); got != "10.0.0.1:6380" {
		t.Fatalf("redisAddr() = %q, want 10.0.0.1:6380", got)
	}
}

func startStream(t *testing.T) (*miniredis.Miniredis, *jobStream) {
	t.Helper()
	mr := miniredis.RunT(t)
	stream := openJobs(mr.Addr())
	t.Cleanup(func() { _ = stream.rdb.Close() })
	return mr, stream
}

func runGroupOnce(t *testing.T, stream *jobStream, cfg WorkerConfig) (JobResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got := make(chan JobResult, 1)
	err := runGroup(ctx, stream, cfg, func(result JobResult) {
		select {
		case got <- result:
		default:
		}
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return JobResult{}, err
	}
	select {
	case result := <-got:
		return result, nil
	default:
		if err != nil {
			return JobResult{}, err
		}
		return JobResult{}, errors.New("runGroup returned without emit")
	}
}
