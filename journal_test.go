package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

const job5Checksum = "c214c3b52b17be575e3dc12a2da1b6f9206ed51807009f7410f53fd00a660851"

func job5ID() uuid.UUID {
	return uuid.MustParse("fcf585ea-56d9-53a7-bd00-6e2560611dc2")
}

func completedResult(t *testing.T, id string, result RedactResult, created, done time.Time) JobResult {
	t.Helper()
	m := newJobMachine()
	m.pickup(created)
	m.complete(done)
	return JobResult{ID: id, Result: result, History: m.history()}
}

func failedResult(t *testing.T, id string, result RedactResult, created, done time.Time) JobResult {
	t.Helper()
	m := newJobMachine()
	m.pickup(created)
	m.fail(done)
	return JobResult{ID: id, Result: result, History: m.history()}
}

func m1Result(t *testing.T) RedactResult {
	t.Helper()
	want, err := RedactPII(m1Fixture)
	if err != nil {
		t.Fatal(err)
	}
	return want
}

func TestDurableIDForJob5(t *testing.T) {
	if got := DurableIDFor("job-5"); got != job5ID() {
		t.Fatalf("DurableIDFor(job-5) = %s, want %s", got, job5ID())
	}
}

func TestDurableIDForRemapsUUIDShapedRedisID(t *testing.T) {
	shaped := job5ID().String()
	got := DurableIDFor(shaped)
	if got == job5ID() {
		t.Fatalf("DurableIDFor(%q) reused the Redis string as jobs.id", shaped)
	}
}

func TestChecksumOfJob5(t *testing.T) {
	if got := ChecksumOf(m1Result(t)); got != job5Checksum {
		t.Fatalf("ChecksumOf(m1Fixture) = %s, want %s", got, job5Checksum)
	}
}

func TestFormatJobLineSharesChecksumBytes(t *testing.T) {
	want := m1Result(t)
	line := formatJobLine(JobResult{ID: "job-5", Result: want})
	prefix := "job-5 "
	if !strings.HasPrefix(line, prefix) {
		t.Fatalf("formatJobLine = %q, want prefix %q", line, prefix)
	}
	payload := strings.TrimPrefix(line, prefix)
	sum := sha256.Sum256([]byte(payload))
	if hex.EncodeToString(sum[:]) != job5Checksum {
		t.Fatalf("formatJobLine JSON checksum = %s, want %s", hex.EncodeToString(sum[:]), job5Checksum)
	}
}

func TestParseTerminalRejectsNonTerminal(t *testing.T) {
	m := newJobMachine()
	m.pickup(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	_, err := parseTerminal(JobResult{ID: "job-5", History: m.history()})
	if !errors.Is(err, errNotTerminal) {
		t.Fatalf("err = %v, want errNotTerminal", err)
	}
}

func TestMemoryJournalIdempotentTimestamps(t *testing.T) {
	ctx := context.Background()
	j := NewMemoryJournal()
	want := m1Result(t)
	firstCreated := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	firstDone := time.Date(2020, 1, 1, 0, 0, 1, 0, time.UTC)
	replayCreated := time.Date(2021, 6, 2, 3, 4, 5, 0, time.UTC)
	replayDone := time.Date(2021, 6, 2, 3, 4, 6, 0, time.UTC)

	if err := j.Record(ctx, completedResult(t, "job-5", want, firstCreated, firstDone)); err != nil {
		t.Fatal(err)
	}
	if err := j.Record(ctx, completedResult(t, "job-5", want, replayCreated, replayDone)); err != nil {
		t.Fatal(err)
	}
	row, err := j.Load(ctx, "job-5")
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != job5ID() {
		t.Fatalf("ID = %s, want %s", row.ID, job5ID())
	}
	if row.JobType != jobTypeRedact {
		t.Fatalf("JobType = %q, want redact", row.JobType)
	}
	if row.FinalState != COMPLETED {
		t.Fatalf("FinalState = %s, want COMPLETED", row.FinalState)
	}
	if row.Checksum != job5Checksum {
		t.Fatalf("Checksum = %s, want %s", row.Checksum, job5Checksum)
	}
	if !row.CreatedAt.Equal(firstCreated) || !row.CompletedAt.Equal(firstDone) {
		t.Fatalf("times = %s %s, want first insert %s %s", row.CreatedAt, row.CompletedAt, firstCreated, firstDone)
	}
}

func TestMemoryJournalConflict(t *testing.T) {
	ctx := context.Background()
	j := NewMemoryJournal()
	want := m1Result(t)
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	done := time.Date(2020, 1, 1, 0, 0, 1, 0, time.UTC)
	if err := j.Record(ctx, completedResult(t, "job-5", want, created, done)); err != nil {
		t.Fatal(err)
	}
	other := want
	other.Counts.EmailsRedacted++
	err := j.Record(ctx, completedResult(t, "job-5", other, created, done))
	if !errors.Is(err, errJournalConflict) {
		t.Fatalf("checksum conflict err = %v, want errJournalConflict", err)
	}
	err = j.Record(ctx, failedResult(t, "job-5", want, created, done))
	if !errors.Is(err, errJournalConflict) {
		t.Fatalf("state conflict err = %v, want errJournalConflict", err)
	}
	row, err := j.Load(ctx, "job-5")
	if err != nil {
		t.Fatal(err)
	}
	if row.Checksum != job5Checksum || row.FinalState != COMPLETED {
		t.Fatalf("row after conflict = %+v", row)
	}
}

func TestMemoryJournalFailedStoresChecksum(t *testing.T) {
	ctx := context.Background()
	j := NewMemoryJournal()
	want := m1Result(t)
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	done := time.Date(2020, 1, 1, 0, 0, 1, 0, time.UTC)
	if err := j.Record(ctx, failedResult(t, "job-5", want, created, done)); err != nil {
		t.Fatal(err)
	}
	row, err := j.Load(ctx, "job-5")
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != FAILED {
		t.Fatalf("FinalState = %s, want FAILED", row.FinalState)
	}
	if row.Checksum != job5Checksum {
		t.Fatalf("FAILED checksum = %s, want %s", row.Checksum, job5Checksum)
	}
}

func TestMemoryJournalLoadUnknown(t *testing.T) {
	_, err := NewMemoryJournal().Load(context.Background(), "job-5")
	if !errors.Is(err, errUnknownStoredJob) {
		t.Fatalf("err = %v, want errUnknownStoredJob", err)
	}
}

func TestOpenJournalEmptyDSN(t *testing.T) {
	_, err := OpenJournal("")
	if !errors.Is(err, errMissingDSN) {
		t.Fatalf("err = %v, want errMissingDSN", err)
	}
}

func TestPostgresJournalOptional(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN is empty")
	}
	ctx := context.Background()
	j, err := OpenJournal(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	want := m1Result(t)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	done := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)
	redisID := "job-live-" + t.Name()
	jr := completedResult(t, redisID, want, created, done)
	if err := j.Record(ctx, jr); err != nil {
		t.Fatal(err)
	}
	if err := j.Record(ctx, jr); err != nil {
		t.Fatal(err)
	}
	row, err := j.Load(ctx, redisID)
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != DurableIDFor(redisID) {
		t.Fatalf("ID = %s, want %s", row.ID, DurableIDFor(redisID))
	}
	if row.Checksum != job5Checksum {
		t.Fatalf("Checksum = %s, want %s", row.Checksum, job5Checksum)
	}
	if row.FinalState != COMPLETED {
		t.Fatalf("FinalState = %s, want COMPLETED", row.FinalState)
	}
}
