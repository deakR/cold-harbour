package queue

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"coldharbour/internal/checkpoint"
	"coldharbour/internal/events"
	"coldharbour/internal/journal"
	"coldharbour/internal/redact"
	"coldharbour/internal/seal"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testSigning(t *testing.T) (ed25519.PrivateKey, seal.ReceiptStore) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, seal.NewMemoryReceiptStore()
}


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

	job, err = parseJob(entry, map[string]string{
		"id": "job-t", "input": "hello", "tenant_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.TenantID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("TenantID = %q, want seeded tenant-a", job.TenantID)
	}

	job, err = parseJob(entry, map[string]string{"id": "job-t", "input": "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if job.TenantID != "" {
		t.Fatalf("TenantID = %q, want empty when tenant_id absent", job.TenantID)
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
	if c.crash != checkpoint.CrashAfterStep1 {
		t.Fatalf("crash = %v, want CrashAfterStep1", c.crash)
	}

	c, err = parseClaim(entry, map[string]string{"input": "hello", "simulateCrashAtStep": "2"})
	if err != nil {
		t.Fatal(err)
	}
	if c.crash != checkpoint.CrashNever {
		t.Fatalf("crash = %v, want CrashNever", c.crash)
	}

	c, err = parseClaim(entry, map[string]string{"id": "fail-1", "input": "hello", "simulateFailure": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.fail {
		t.Fatal("fail = false, want true")
	}
	if c.job != (Job{ID: "fail-1", Input: "hello"}) {
		t.Fatalf("job = %+v", c.job)
	}
	if c.fields["simulateFailure"] != "1" {
		t.Fatalf("fields = %#v, want simulateFailure=1", c.fields)
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

	want, err := redact.RedactPII(input)
	if err != nil {
		t.Fatal(err)
	}

	got, err := runGroupOnce(t, stream, testWorkerConfig("sit"), journal.NewMemoryJournal(), seal.NewMemoryKeyStore())
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

	store := journal.NewMemoryJournal()
	got, err := runGroupOnce(t, stream, testWorkerConfig("skip"), store, seal.NewMemoryKeyStore())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "good" {
		t.Fatalf("ID = %q, want good", got.ID)
	}
	if _, err := store.Load(ctx, "bad"); !errors.Is(err, journal.ErrUnknownStoredJob) {
		t.Fatalf("poison Load err = %v, want ErrUnknownStoredJob", err)
	}
	row, err := store.Load(ctx, "good")
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.COMPLETED {
		t.Fatalf("good FinalState = %s, want COMPLETED", row.FinalState)
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
	if err := stream.enqueue(ctx, Job{ID: "crash-1", Input: redact.M1Fixture}, checkpoint.CrashAfterStep1); err != nil {
		t.Fatal(err)
	}

	store := journal.NewMemoryJournal()
	keys := seal.NewMemoryKeyStore()
	priv, receipts := testSigning(t)
	err := RunGroup(ctx, stream, cfg1, store, keys, receipts, priv, func(journal.JobResult) {})
	if !errors.Is(err, checkpoint.ErrSimulatedCrash) {
		t.Fatalf("worker 1 err = %v, want ErrSimulatedCrash", err)
	}
	if _, err := store.Load(ctx, "crash-1"); !errors.Is(err, journal.ErrUnknownStoredJob) {
		t.Fatalf("Load after crash err = %v, want ErrUnknownStoredJob", err)
	}

	rawPartial, err := stream.rdb.HGet(ctx, memKey("crash-1"), memFieldPartial).Result()
	if err != nil {
		t.Fatal(err)
	}
	if json.Valid([]byte(rawPartial)) {
		t.Fatalf("partial_result after crash is JSON: %s", rawPartial)
	}
	var asJSON redact.RedactResult
	if err := json.Unmarshal([]byte(rawPartial), &asJSON); err == nil {
		t.Fatalf("partial_result unmarshaled as plaintext: %+v", asJSON)
	}

	pending, err := stream.rdb.XPending(ctx, jobsStreamKey, workerGroup).Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 1 {
		t.Fatalf("pending after crash = %d, want 1", pending.Count)
	}

	mr.SetTime(t0.Add(cfg1.Idle))

	got, err := runGroupOnce(t, stream, cfg2, store, keys)
	if err != nil {
		t.Fatal(err)
	}
	want, err := redact.RedactPII(redact.M1Fixture)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "crash-1" {
		t.Fatalf("ID = %q, want crash-1", got.ID)
	}
	if got.Result != want {
		t.Fatalf("Result = %+v, want %+v", got.Result, want)
	}

	exists, err := stream.rdb.Exists(ctx, memKey("crash-1")).Result()
	if err != nil {
		t.Fatal(err)
	}
	if exists != 0 {
		t.Fatalf("mem hash after COMPLETED exists=%d, want 0", exists)
	}

	pending, err = stream.rdb.XPending(ctx, jobsStreamKey, workerGroup).Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 0 {
		t.Fatalf("pending after reclaim = %d, want 0", pending.Count)
	}

	row, err := store.Load(ctx, "crash-1")
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.COMPLETED {
		t.Fatalf("FinalState = %s, want COMPLETED", row.FinalState)
	}
	if row.Checksum != journal.ChecksumOf(want) {
		t.Fatalf("Checksum = %s, want %s", row.Checksum, journal.ChecksumOf(want))
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
	if got := RedisAddr(); got != "127.0.0.1:6379" {
		t.Fatalf("default redisAddr() = %q, want 127.0.0.1:6379", got)
	}
	t.Setenv("REDIS_ADDR", "10.0.0.1:6380")
	if got := RedisAddr(); got != "10.0.0.1:6380" {
		t.Fatalf("redisAddr() = %q, want 10.0.0.1:6380", got)
	}
}

func TestSuccessfulJobPublishesCompletedEvent(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	tenant := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	jobID := "job-evt"

	sub := stream.rdb.Subscribe(ctx, events.Channel)
	t.Cleanup(func() { _ = sub.Close() })
	if _, err := sub.Receive(ctx); err != nil {
		t.Fatal(err)
	}
	msgCh := sub.Channel()

	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.Add(ctx, Job{ID: jobID, Input: "Email a@b.com x", TenantID: tenant}); err != nil {
		t.Fatal(err)
	}

	got, err := runGroupOnce(t, stream, testWorkerConfig("evt"), journal.NewMemoryJournal(), seal.NewMemoryKeyStore())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != jobID {
		t.Fatalf("ID = %q, want %s", got.ID, jobID)
	}

	var completedPayloads []string
	deadline := time.After(2 * time.Second)
loop:
	for {
		select {
		case msg := <-msgCh:
			var e events.JobEvent
			if err := json.Unmarshal([]byte(msg.Payload), &e); err != nil {
				t.Fatalf("unmarshal %q: %v", msg.Payload, err)
			}
			if e.Status == string(journal.COMPLETED) {
				completedPayloads = append(completedPayloads, msg.Payload)
			}
			if len(completedPayloads) > 0 {
				select {
				case extra := <-msgCh:
					var e2 events.JobEvent
					if err := json.Unmarshal([]byte(extra.Payload), &e2); err != nil {
						t.Fatalf("unmarshal %q: %v", extra.Payload, err)
					}
					if e2.Status == string(journal.COMPLETED) {
						completedPayloads = append(completedPayloads, extra.Payload)
					}
				case <-time.After(50 * time.Millisecond):
				}
				break loop
			}
		case <-deadline:
			break loop
		}
	}
	if len(completedPayloads) != 1 {
		t.Fatalf("COMPLETED events = %d, want 1; got %v", len(completedPayloads), completedPayloads)
	}
	payload := completedPayloads[0]
	if !strings.Contains(payload, tenant) {
		t.Fatalf("payload %s missing tenant id %s", payload, tenant)
	}
	if !strings.Contains(payload, jobID) {
		t.Fatalf("payload %s missing job id %s", payload, jobID)
	}
}

func startStream(t *testing.T) (*miniredis.Miniredis, *jobStream) {
	t.Helper()
	mr := miniredis.RunT(t)
	stream := OpenJobs(mr.Addr())
	t.Cleanup(func() { _ = stream.rdb.Close() })
	return mr, stream
}

func TestRunGroupRecordsThenEmitsThenAcks(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.Add(ctx, Job{ID: "job-cli", Input: "Email a@b.com x"}); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	emitSawRow := false
	emitSawPending := false
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	priv, receipts := testSigning(t)
	err := RunGroup(runCtx, stream, testWorkerConfig("order"), store, seal.NewMemoryKeyStore(), receipts, priv, func(result journal.JobResult) {
		if result.ID != "job-cli" {
			t.Errorf("emit ID = %q, want job-cli", result.ID)
		}
		if _, loadErr := store.Load(ctx, "job-cli"); loadErr != nil {
			t.Errorf("Load at emit err = %v, want row", loadErr)
		} else {
			emitSawRow = true
		}
		pending, pendErr := stream.rdb.XPending(ctx, jobsStreamKey, workerGroup).Result()
		if pendErr != nil {
			t.Errorf("XPending at emit: %v", pendErr)
		} else if pending.Count != 1 {
			t.Errorf("pending at emit = %d, want 1", pending.Count)
		} else {
			emitSawPending = true
		}
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if !emitSawRow || !emitSawPending {
		t.Fatalf("emitSawRow=%v emitSawPending=%v", emitSawRow, emitSawPending)
	}
	pending, err := stream.rdb.XPending(ctx, jobsStreamKey, workerGroup).Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 0 {
		t.Fatalf("pending after handle = %d, want 0", pending.Count)
	}
}

func TestRunGroupNilJournalPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("runGroup(nil journal) did not panic")
		}
	}()
	priv, receipts := testSigning(t)
	_ = RunGroup(context.Background(), nil, testWorkerConfig("nil"), nil, nil, receipts, priv, func(journal.JobResult) {})
}

func TestRunGroupNilKeyStorePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("runGroup(nil keys) did not panic")
		}
	}()
	priv, receipts := testSigning(t)
	_ = RunGroup(context.Background(), nil, testWorkerConfig("nil-keys"), journal.NewMemoryJournal(), nil, receipts, priv, func(journal.JobResult) {})
}

func TestRecordErrorSkipsEmitAndAck(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.Add(ctx, Job{ID: "job-cli", Input: "Email a@b.com x"}); err != nil {
		t.Fatal(err)
	}
	forced := errors.New("forced record failure")
	emitted := false
	priv, receipts := testSigning(t)
	err := RunGroup(ctx, stream, testWorkerConfig("rec"), errJournal{err: forced}, seal.NewMemoryKeyStore(), receipts, priv, func(journal.JobResult) {
		emitted = true
	})
	if !errors.Is(err, forced) {
		t.Fatalf("err = %v, want forced record failure", err)
	}
	if emitted {
		t.Fatal("emit ran after Record error")
	}
	pending, err := stream.rdb.XPending(ctx, jobsStreamKey, workerGroup).Result()
	if err != nil {
		t.Fatal(err)
	}
	if pending.Count != 1 {
		t.Fatalf("pending after Record error = %d, want 1", pending.Count)
	}
}

func TestOutputsSurviveRedisFlushAll(t *testing.T) {
	mr, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	input := "Email alice.nguyen@school.edu about the lab report."
	if err := stream.Add(ctx, Job{ID: "job-cli", Input: input}); err != nil {
		t.Fatal(err)
	}
	store := journal.NewMemoryJournal()
	got, err := runGroupOnce(t, stream, testWorkerConfig("flush"), store, seal.NewMemoryKeyStore())
	if err != nil {
		t.Fatal(err)
	}
	want, err := redact.RedactPII(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != want {
		t.Fatalf("Result = %+v, want %+v", got.Result, want)
	}
	mr.FlushAll()
	body, err := store.LoadOutput(ctx, "job-cli")
	if err != nil {
		t.Fatal(err)
	}
	if body != string(redact.MarshalResult(want)) {
		t.Fatalf("outputs.body after FLUSHALL = %q, want MarshalResult", body)
	}
	row, err := store.Load(ctx, "job-cli")
	if err != nil {
		t.Fatal(err)
	}
	if row.Checksum != journal.ChecksumOf(want) {
		t.Fatalf("jobs.output_checksum after FLUSHALL = %s", row.Checksum)
	}
	if row.Checksum == body {
		t.Fatal("jobs.output_checksum stored the JSON")
	}
}

type errJournal struct{ err error }

func (j errJournal) Record(context.Context, journal.JobResult) error { return j.err }

func (j errJournal) Load(context.Context, string) (journal.StoredJob, error) {
	return journal.StoredJob{}, j.err
}

func TestSuccessfulJobDeletesMemHash(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.Add(ctx, Job{ID: "job-purge", Input: "Email a@b.com x"}); err != nil {
		t.Fatal(err)
	}
	keys := seal.NewMemoryKeyStore()
	receipts := seal.NewMemoryReceiptStore()
	priv, _ := testSigning(t)
	got, err := func() (journal.JobResult, error) {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		ch := make(chan journal.JobResult, 1)
		err := RunGroup(ctx, stream, testWorkerConfig("purge-ok"), journal.NewMemoryJournal(), keys, receipts, priv, func(r journal.JobResult) {
			ch <- r
			cancel()
		})
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return journal.JobResult{}, err
		}
		select {
		case r := <-ch:
			return r, nil
		default:
			return journal.JobResult{}, errors.New("no emit")
		}
	}()
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "job-purge" {
		t.Fatalf("ID = %q", got.ID)
	}
	exists, err := stream.rdb.Exists(ctx, memKey("job-purge")).Result()
	if err != nil {
		t.Fatal(err)
	}
	if exists != 0 {
		t.Fatalf("EXISTS mem = %d, want 0", exists)
	}
	rec, ok, err := receipts.Get(ctx, journal.DurableIDFor("job-purge"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("missing purge receipt")
	}
	pub := priv.Public().(ed25519.PublicKey)
	msg, err := seal.PurgeMessage("job-purge", rec.PurgedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !seal.Verify(pub, msg, rec.Signature) {
		t.Fatal("receipt signature invalid")
	}
	if _, err := keys.Ensure(ctx, journal.DurableIDFor("job-purge")); err == nil {
		t.Fatal("Ensure after Destroy succeeded, want errKeyDestroyed")
	}
}

func TestCrashLeavesMemHash(t *testing.T) {
	_, stream := startStream(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	if err := stream.enqueue(ctx, Job{ID: "crash-keep", Input: redact.M1Fixture}, checkpoint.CrashAfterStep1); err != nil {
		t.Fatal(err)
	}
	priv, receipts := testSigning(t)
	err := RunGroup(ctx, stream, testWorkerConfig("crash-keep"), journal.NewMemoryJournal(), seal.NewMemoryKeyStore(), receipts, priv, func(journal.JobResult) {
		t.Error("emit after crash")
	})
	if !errors.Is(err, checkpoint.ErrSimulatedCrash) {
		t.Fatalf("err = %v, want ErrSimulatedCrash", err)
	}
	exists, err := stream.rdb.Exists(ctx, memKey("crash-keep")).Result()
	if err != nil {
		t.Fatal(err)
	}
	if exists != 1 {
		t.Fatalf("EXISTS mem after crash = %d, want 1", exists)
	}
	if _, ok, err := receipts.Get(ctx, journal.DurableIDFor("crash-keep")); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("purge receipt after crash, want none")
	}
}

func runGroupOnce(t *testing.T, stream *jobStream, cfg WorkerConfig, store journal.Journal, keys seal.KeyStore) (journal.JobResult, error) {
	t.Helper()
	priv, receipts := testSigning(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got := make(chan journal.JobResult, 1)
	err := RunGroup(ctx, stream, cfg, store, keys, receipts, priv, func(result journal.JobResult) {
		select {
		case got <- result:
		default:
		}
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		return journal.JobResult{}, err
	}
	select {
	case result := <-got:
		return result, nil
	default:
		if err != nil {
			return journal.JobResult{}, err
		}
		return journal.JobResult{}, errors.New("runGroup returned without emit")
	}
}
