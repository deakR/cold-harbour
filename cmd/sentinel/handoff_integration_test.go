package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"coldharbour/internal/api"
	"coldharbour/internal/journal"
	"coldharbour/internal/keys"
	"coldharbour/internal/migrate"
	"coldharbour/internal/policy"
	"coldharbour/internal/policystore"
	"coldharbour/internal/queue"
	"coldharbour/internal/redact"
	"coldharbour/internal/runner"
	"coldharbour/internal/seal"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

func TestHeldJobCompletesWithReceipt(t *testing.T) {
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		t.Skip("POSTGRES_DSN is empty")
	}

	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate.Up(sqldb); err != nil {
		t.Fatal(err)
	}
	_ = sqldb.Close()

	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	keyStore := seal.NewMemoryKeyStore()
	stream := queue.OpenJobs(mr.Addr())
	apiSrv := api.New(pool, rdb, keyStore, stream, []string{"redact"})
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)

	tenant, adminKey := createTenantAndKey(t, pool, "admin")
	sentinelKey := createKeyOnTenant(t, pool, tenant, "sentinel")
	appKey := createKeyOnTenant(t, pool, tenant, "app")

	putPolicy(t, srv.URL, adminKey, map[string]any{
		"detectors":      []string{"email"},
		"mode":           "redact",
		"failPolicy":     "closed",
		"maxInlineBytes": 8,
	})

	jstore, err := journal.OpenJournal(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = jstore.Close() })
	receipts, err := seal.OpenPostgresReceiptStore(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = receipts.Close() })
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.PrepareGroup(context.Background(), stream); err != nil {
		t.Fatal(err)
	}
	wctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errCh := make(chan error, 1)
	go func() {
		errCh <- queue.RunGroup(wctx, stream, queue.WorkerConfig{
			Consumer: "s4-held",
			Idle:     50 * time.Millisecond,
			Poll:     20 * time.Millisecond,
		}, queue.Deps{
			Registry: runner.NewRegistry(redact.Runner{Load: func(ctx context.Context, tenantID string) (policy.Doc, bool, error) {
				id, err := uuid.Parse(tenantID)
				if err != nil {
					return policy.Doc{}, false, err
				}
				return policystore.Load(ctx, pool, id)
			}}),
			Journal:    jstore,
			Keys:       keyStore,
			Receipts:   receipts,
			SigningKey: priv,
		})
	}()

	bin := buildSentinel(t)
	secret := "jane.doe@example.com"
	long := secret + strings.Repeat("z", 40)
	addr := freeAddr(t)
	cmd := exec.Command(bin, "run",
		"--in", "-", "--out", "-",
		"--metrics", addr,
		"--control-plane", srv.URL,
		"--state-dir", t.TempDir(),
		"--policy-interval", "1h",
	)
	cmd.Env = append(os.Environ(), "SENTINEL_API_KEY="+sentinelKey)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var out safeBuf
	var stderr safeBuf
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	go func() { _, _ = io.Copy(&out, stdout) }()
	if _, err := io.WriteString(stdin, long+"\n"); err != nil {
		t.Fatal(err)
	}

	held := waitHeldLine(t, &out)
	if strings.Contains(held, secret) || strings.Contains(stderr.String(), secret) {
		t.Fatal("held path exposed the raw value")
	}
	jobID := strings.TrimPrefix(strings.TrimSpace(held), "[HELD job=")
	jobID = strings.TrimSuffix(jobID, "]")
	if jobID == "" {
		t.Fatalf("bad held line %q", held)
	}

	var source string
	if err := pool.QueryRow(context.Background(), `SELECT source FROM job_accepts WHERE redis_job_id = $1`, jobID).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "sentinel" {
		t.Fatalf("source = %q", source)
	}

	result := waitJobCompleted(t, srv.URL, appKey, jobID)
	rawResult, _ := json.Marshal(result["result"])
	var parsed map[string]any
	if err := json.Unmarshal(rawResult, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed["policyVersion"] != float64(1) {
		t.Fatalf("policyVersion = %v", parsed["policyVersion"])
	}
	text, _ := parsed["redactedText"].(string)
	if strings.Contains(text, secret) {
		t.Fatal("job result kept the raw value")
	}

	durable := journal.DurableIDFor(jobID)
	var purgedAt time.Time
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		err = pool.QueryRow(context.Background(), `SELECT purged_at FROM purge_receipts WHERE job_id = $1`, durable).Scan(&purgedAt)
		if err == nil && !purgedAt.IsZero() {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("purge receipt: %v", err)
	}
	if purgedAt.IsZero() {
		t.Fatal("purge receipt missing timestamp")
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
	}
}

func createTenantAndKey(t *testing.T, pool *pgxpool.Pool, role string) (uuid.UUID, string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var tenant uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "s4-"+uuid.NewString()).Scan(&tenant); err != nil {
		t.Fatal(err)
	}
	_, _, full, err := keys.Create(ctx, tx, tenant, role, role)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return tenant, full
}

func createKeyOnTenant(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID, role string) string {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	_, _, full, err := keys.Create(ctx, tx, tenant, role, role)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return full
}

func putPolicy(t *testing.T, base, key string, body map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, base+"/v1/policy", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("PUT policy = %d %s", res.StatusCode, b)
	}
}

func waitHeldLine(t *testing.T, out *safeBuf) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		text := out.String()
		if strings.Contains(text, "[HELD job=") {
			return strings.Split(text, "\n")[0]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no held line in %q", out.String())
	return ""
}

func waitJobCompleted(t *testing.T, base, key, jobID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, base+"/v1/jobs/"+jobID, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-API-Key", key)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			time.Sleep(30 * time.Millisecond)
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got["status"] == "COMPLETED" {
			return got
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("job did not complete")
	return nil
}
