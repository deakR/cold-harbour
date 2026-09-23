package queue

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"coldharbour/internal/journal"
	"coldharbour/internal/redact"
	"coldharbour/internal/seal"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

func TestIntegrationCrashAndBury(t *testing.T) {
	if os.Getenv("COLDHARBOUR_INTEGRATION") == "" || os.Getenv("POSTGRES_DSN") == "" {
		t.Skip("set COLDHARBOUR_INTEGRATION=1 and POSTGRES_DSN")
	}
	dsn := os.Getenv("POSTGRES_DSN")
	opt := redisOptions(RedisAddr())
	opt.DB = 14
	rdb := redis.NewClient(opt)
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	n, err := rdb.DBSize(ctx).Result()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		_ = rdb.Close()
		t.Skip("redis database 14 is not empty")
	}
	t.Cleanup(func() {
		_ = rdb.FlushDB(context.Background()).Err()
		_ = rdb.Close()
	})
	stream := &jobStream{rdb: rdb}
	if err := stream.ensureGroup(ctx); err != nil {
		t.Fatal(err)
	}
	store, err := journal.OpenJournal(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	receipts, err := seal.OpenPostgresReceiptStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = receipts.Close() })
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tenant := uuid.New()
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants (id, name) VALUES ($1, $2)`, tenant, "crash-"+tenant.String()); err != nil {
		t.Fatal(err)
	}
	priv, _ := testSigning(t)

	runCrash := func(name string, set func(jobID string) Hooks) {
		t.Run(name, func(t *testing.T) {
			jobID := name + "-" + uuid.NewString()
			keys := seal.NewMemoryKeyStore()
			if err := stream.Add(ctx, keys, Job{ID: jobID, Input: redact.M1Fixture, TenantID: tenant.String()}); err != nil {
				t.Fatal(err)
			}
			cfg := testWorkerConfig(name + "-1")
			runCtx, cancel := context.WithCancel(context.Background())
			errCh := make(chan error, 1)
			go func() {
				errCh <- RunGroup(runCtx, stream, cfg, testDeps(store, keys, receipts, priv, func(journal.JobResult) {}, set(jobID)))
			}()
			var err error
			select {
			case err = <-errCh:
			case <-time.After(8 * time.Second):
				waiting, _ := stream.Len(context.Background())
				cancel()
				<-errCh
				t.Fatalf("first worker timed out, stream len %d", waiting)
			}
			cancel()
			if !errors.Is(err, ErrSimulatedCrash) {
				t.Fatalf("first worker err = %v, want ErrSimulatedCrash", err)
			}
			time.Sleep(cfg.Idle + 30*time.Millisecond)
			runUntilQuiet(t, stream, testWorkerConfig(name+"-2"), testDeps(store, keys, receipts, priv, func(journal.JobResult) {}, Hooks{}))
			assertPurged(t, ctx, db, stream, keys, receipts, tenant, jobID, true)
		})
	}
	runCrash("step1", func(id string) Hooks {
		return Hooks{CrashAfterStep1: func(jobID string) bool { return jobID == id }}
	})
	runCrash("record", func(id string) Hooks {
		return Hooks{CrashAfterRecord: func(jobID string) bool { return jobID == id }}
	})
	runCrash("keydestroy", func(id string) Hooks {
		return Hooks{CrashAfterKeyDestroy: func(jobID string) bool { return jobID == id }}
	})

	t.Run("unknown-type", func(t *testing.T) {
		jobID := "unknown-type-" + uuid.NewString()
		keys := seal.NewMemoryKeyStore()
		key, err := keys.Ensure(ctx, journal.DurableIDFor(jobID))
		if err != nil {
			t.Fatal(err)
		}
		sealed, err := seal.Seal(key, []byte("hello"))
		if err != nil {
			t.Fatal(err)
		}
		if err := rdb.XAdd(ctx, &redis.XAddArgs{Stream: jobsStreamKey, Values: map[string]any{
			"id": jobID, "input": sealed, "job_type": "nope", "tenant_id": tenant.String(),
		}}).Err(); err != nil {
			t.Fatal(err)
		}
		followUp(t, ctx, db, stream, store, keys, receipts, priv, tenant, jobID, true)
	})
	t.Run("bad-seal", func(t *testing.T) {
		jobID := "bad-seal-" + uuid.NewString()
		keys := seal.NewMemoryKeyStore()
		if _, err := keys.Ensure(ctx, journal.DurableIDFor(jobID)); err != nil {
			t.Fatal(err)
		}
		if err := rdb.XAdd(ctx, &redis.XAddArgs{Stream: jobsStreamKey, Values: map[string]any{
			"id": jobID, "input": "not-a-seal", "job_type": "redact", "tenant_id": tenant.String(),
		}}).Err(); err != nil {
			t.Fatal(err)
		}
		followUp(t, ctx, db, stream, store, keys, receipts, priv, tenant, jobID, true)
	})
	t.Run("missing-key", func(t *testing.T) {
		jobID := "missing-key-" + uuid.NewString()
		keys := seal.NewMemoryKeyStore()
		if err := rdb.XAdd(ctx, &redis.XAddArgs{Stream: jobsStreamKey, Values: map[string]any{
			"id": jobID, "input": "Email a@b.com x", "tenant_id": tenant.String(),
		}}).Err(); err != nil {
			t.Fatal(err)
		}
		followUp(t, ctx, db, stream, store, keys, receipts, priv, tenant, jobID, false)
	})
}

func followUp(t *testing.T, ctx context.Context, db *sql.DB, stream *jobStream, store journal.Journal, keys *seal.MemoryKeyStore, receipts seal.ReceiptStore, priv ed25519.PrivateKey, tenant uuid.UUID, jobID string, hadKey bool) {
	t.Helper()
	good := jobID + "-good"
	if err := stream.Add(ctx, keys, Job{ID: good, Input: "Email a@b.com x", TenantID: tenant.String()}); err != nil {
		t.Fatal(err)
	}
	saw := make(chan string, 1)
	runUntilQuiet(t, stream, testWorkerConfig(jobID), testDeps(store, keys, receipts, priv, func(result journal.JobResult) {
		if result.ID == good {
			select {
			case saw <- result.ID:
			default:
			}
		}
	}, Hooks{}))
	select {
	case id := <-saw:
		if id != good {
			t.Fatalf("emitted %s, want %s", id, good)
		}
	default:
		t.Fatalf("worker stopped before %s", good)
	}
	assertPurged(t, ctx, db, stream, keys, receipts, tenant, jobID, hadKey)
}

func runUntilQuiet(t *testing.T, stream *jobStream, cfg WorkerConfig, deps Deps) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- RunGroup(ctx, stream, cfg, deps)
	}()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-errCh:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("worker err = %v", err)
			}
			return
		case <-ticker.C:
			n, err := stream.Len(context.Background())
			if err == nil && n == 0 {
				cancel()
			}
		case <-deadline:
			t.Fatal("worker did not finish")
		}
	}
}

func assertPurged(t *testing.T, ctx context.Context, db *sql.DB, stream *jobStream, keys *seal.MemoryKeyStore, receipts seal.ReceiptStore, tenant uuid.UUID, jobID string, hadKey bool) {
	t.Helper()
	if n, err := stream.Len(ctx); err != nil || n != 0 {
		t.Fatalf("stream len = %d err=%v, want 0", n, err)
	}
	entries, err := stream.rdb.XRange(ctx, jobsStreamKey, "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if _, ok := entry.Values["input"]; ok {
			t.Fatalf("stream entry %s still has input", entry.ID)
		}
	}
	if exists, err := stream.rdb.Exists(ctx, memKey(jobID)).Result(); err != nil || exists != 0 {
		t.Fatalf("checkpoint %s exists=%d err=%v", jobID, exists, err)
	}
	if _, ok, err := receipts.Get(ctx, journal.DurableIDFor(jobID)); err != nil || !ok {
		t.Fatalf("receipt for %s ok=%v err=%v", jobID, ok, err)
	}
	var ledgerN int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM audit_ledger
		WHERE tenant_id = $1 AND kind = 'purge_receipt' AND payload->>'jobId' = $2
	`, tenant, jobID).Scan(&ledgerN); err != nil {
		t.Fatal(err)
	}
	if ledgerN != 1 {
		t.Fatalf("ledger rows for %s = %d, want 1", jobID, ledgerN)
	}
	_, ok, err := keys.Load(ctx, journal.DurableIDFor(jobID))
	if hadKey {
		if err == nil || ok {
			t.Fatalf("key %s ok=%v err=%v, want destroyed", jobID, ok, err)
		}
		return
	}
	if ok || err != nil {
		t.Fatalf("missing key %s ok=%v err=%v", jobID, ok, err)
	}
}
