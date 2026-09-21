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

func TestStreamPicksUpSittingJob(t *testing.T) {
	mr := miniredis.RunT(t)
	stream := openJobs(mr.Addr())
	t.Cleanup(func() { _ = stream.rdb.Close() })

	input := "Email alice.nguyen@school.edu about the lab report."
	if err := stream.Add(context.Background(), Job{ID: "job-cli", Input: input}); err != nil {
		t.Fatal(err)
	}

	want, err := RedactPII(input)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan JobResult, 1)
	errc := make(chan error, 1)
	go func() {
		errc <- runStream(ctx, stream, func(result JobResult) {
			got <- result
			cancel()
		})
	}()

	select {
	case result := <-got:
		if result.ID != "job-cli" {
			t.Fatalf("ID = %q, want %q", result.ID, "job-cli")
		}
		if result.Result != want {
			t.Fatalf("Result = %+v, want %+v", result.Result, want)
		}
		if result.Result.RedactedText != "Email [EMAIL_REDACTED] about the lab report." {
			t.Fatalf("RedactedText = %q, want Email [EMAIL_REDACTED] about the lab report.", result.Result.RedactedText)
		}
	case err := <-errc:
		t.Fatalf("runStream returned before emit: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sitting job; XREAD likely started at $")
	}

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("runStream err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runStream did not return after cancel")
	}
}

func TestSeedIfEmpty(t *testing.T) {
	mr := miniredis.RunT(t)
	stream := openJobs(mr.Addr())
	t.Cleanup(func() { _ = stream.rdb.Close() })
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

func TestRunStreamSkipsBadEntry(t *testing.T) {
	mr := miniredis.RunT(t)
	stream := openJobs(mr.Addr())
	t.Cleanup(func() { _ = stream.rdb.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	raw := redis.NewClient(&redis.Options{Addr: mr.Addr()})
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

	got := make(chan JobResult, 1)
	go func() {
		_ = runStream(ctx, stream, func(result JobResult) {
			got <- result
		})
	}()

	select {
	case result := <-got:
		cancel()
		if result.ID != "good" {
			t.Fatalf("ID = %q, want good", result.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not skip bad entry")
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
