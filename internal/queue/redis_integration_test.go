package queue

import (
	"context"
	"os"
	"strings"
	"testing"

	"coldharbour/internal/journal"
	"coldharbour/internal/redact"
	"coldharbour/internal/seal"

	"github.com/redis/go-redis/v9"
)

func TestBuryStripsInputOnRealRedis(t *testing.T) {
	stream := openRealRedis(t)
	ctx := context.Background()
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	job := Job{ID: "job-bury-real", Input: redact.M1Fixture, TenantID: testTenantID}
	store := journal.NewMemoryJournal()
	keys := seal.NewMemoryKeyStore()
	if err := addFailJob(ctx, stream, keys, job); err != nil {
		t.Fatal(err)
	}
	priv, receipts := testSigning(t)

	if err := consumeOne(t, stream, store, keys, receipts, priv, nil, failThese(job.ID)); err != nil {
		t.Fatal(err)
	}
	assertMainLen(t, stream, 1)
	assertSealedInput(t, stream, keys, job.ID, redact.M1Fixture)
	assertDLQLen(t, stream, 0)

	if err := consumeOne(t, stream, store, keys, receipts, priv, nil, failThese(job.ID)); err != nil {
		t.Fatal(err)
	}
	assertMainLen(t, stream, 1)
	assertSealedInput(t, stream, keys, job.ID, redact.M1Fixture)

	if err := consumeOne(t, stream, store, keys, receipts, priv, nil, failThese(job.ID)); err != nil {
		t.Fatal(err)
	}
	assertMainLen(t, stream, 0)
	assertPending(t, stream, 0)
	assertDLQLen(t, stream, 1)
	assertStreamField(t, stream, dlqStreamKey, 0, "id", job.ID)
	assertNoStreamField(t, stream, dlqStreamKey, 0, "input")
	row, err := store.Load(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.FinalState != journal.FAILED {
		t.Fatalf("FinalState = %s, want FAILED", row.FinalState)
	}
}

func assertSealedInput(t *testing.T, stream *jobStream, keys seal.KeyStore, jobID, plain string) {
	t.Helper()
	entries, err := stream.rdb.XRange(context.Background(), jobsStreamKey, "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("XRANGE len = %d, want 1", len(entries))
	}
	fields := valuesToFields(entries[0].Values)
	if fields["input"] == "" || strings.Contains(fields["input"], plain) {
		t.Fatal("stream entry does not hold sealed input")
	}
	key, err := keys.Ensure(context.Background(), journal.DurableIDFor(jobID))
	if err != nil {
		t.Fatal(err)
	}
	got, err := seal.Open(key, fields["input"])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != plain {
		t.Fatal("sealed input did not open to the queued text")
	}
}

func openRealRedis(t *testing.T) *jobStream {
	t.Helper()
	if os.Getenv("COLDHARBOUR_INTEGRATION") == "" {
		t.Skip("set COLDHARBOUR_INTEGRATION=1 to run against Redis")
	}
	opt := redisOptions(RedisAddr())
	opt.DB = 15
	stream := &jobStream{rdb: redis.NewClient(opt)}
	ctx := context.Background()
	if err := stream.rdb.Ping(ctx).Err(); err != nil {
		_ = stream.rdb.Close()
		t.Fatal(err)
	}
	n, err := stream.rdb.DBSize(ctx).Result()
	if err != nil {
		_ = stream.rdb.Close()
		t.Fatal(err)
	}
	if n != 0 {
		_ = stream.rdb.Close()
		t.Skip("redis database 15 is not empty")
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		for _, key := range []string{jobsStreamKey, dlqStreamKey, retryKey("job-bury-real"), memKey("job-bury-real")} {
			_ = stream.rdb.Del(cleanupCtx, key).Err()
		}
		_ = stream.rdb.Close()
	})
	return stream
}
