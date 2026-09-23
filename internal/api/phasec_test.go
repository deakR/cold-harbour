package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"coldharbour/internal/keys"
	"coldharbour/internal/ledger"
	"coldharbour/internal/migrate"
	"coldharbour/internal/queue"
	"coldharbour/internal/seal"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

func phaseCDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
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
	if err := migrate.Up(sqldb); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	_ = sqldb.Close()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func phaseCServer(t *testing.T, pool *pgxpool.Pool) *httptest.Server {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	srv := New(pool, rdb, seal.NewMemoryKeyStore(), queue.OpenJobs(mr.Addr()), []string{"redact", "mask"})
	return httptest.NewServer(srv.Handler())
}

func createTenantKey(t *testing.T, pool *pgxpool.Pool, role string) (uuid.UUID, string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	var tenant uuid.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "t-"+role+"-"+uuid.NewString()).Scan(&tenant); err != nil {
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

func authed(req *http.Request, key string) *http.Request {
	req.Header.Set("X-API-Key", key)
	return req
}

func TestFreshDatabaseRejectsDemoKey(t *testing.T) {
	pool := phaseCDB(t)
	srv := phaseCServer(t, pool)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	authed(req, "ch_live_a_demo_key_aaaaaaaa")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}

func TestCreatedKeyWorksUntilRevoke(t *testing.T) {
	pool := phaseCDB(t)
	srv := phaseCServer(t, pool)
	defer srv.Close()
	_, admin := createTenantKey(t, pool, "admin")
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/keys", jsonBody(t, map[string]string{"name": "app", "role": "app"}))
	authed(req, admin)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("create = %d %s", res.StatusCode, b)
	}
	var created struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	}
	if err := json.NewDecoder(res.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	authed(req, created.Key)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("app list = %d, want 200", res.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodDelete, srv.URL+"/v1/keys/"+created.ID, nil)
	authed(req, admin)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("revoke = %d, want 204", res.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	authed(req, created.Key)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked = %d, want 401", res.StatusCode)
	}
}

func TestAppCannotCreateKeysAndSentinelCannotReadReport(t *testing.T) {
	pool := phaseCDB(t)
	srv := phaseCServer(t, pool)
	defer srv.Close()
	_, appKey := createTenantKey(t, pool, "app")
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/keys", jsonBody(t, map[string]string{"name": "x", "role": "app"}))
	authed(req, appKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("app create key = %d, want 403", res.StatusCode)
	}
	_, sentinel := createTenantKey(t, pool, "sentinel")
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/reports/compliance?from=2000-01-01T00:00:00Z&to=2100-01-01T00:00:00Z", nil)
	authed(req, sentinel)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("sentinel report = %d, want 403", res.StatusCode)
	}
}

func TestTenantKeyCannotReadAnotherTenant(t *testing.T) {
	pool := phaseCDB(t)
	srv := phaseCServer(t, pool)
	defer srv.Close()
	tenantA, keyA := createTenantKey(t, pool, "admin")
	_, keyB := createTenantKey(t, pool, "admin")
	jobID := uuid.NewString()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO job_accepts (redis_job_id, tenant_id, accepted_at) VALUES ($1, $2, now())
	`, jobID, tenantA); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs/"+jobID, nil)
	authed(req, keyB)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-tenant = %d, want 404", res.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs/"+jobID, nil)
	authed(req, keyA)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("owner = %d, want 200", res.StatusCode)
	}
}

func TestLedgerVerifyAndConcurrentAppend(t *testing.T) {
	pool := phaseCDB(t)
	ctx := context.Background()
	tenant, _ := createTenantKey(t, pool, "admin")
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tx, err := pool.Begin(ctx)
			if err != nil {
				errCh <- err
				return
			}
			defer tx.Rollback(ctx)
			payload, _ := json.Marshal(map[string]int{"n": n, "r": int(time.Now().UnixNano())})
			if err := ledger.Append(ctx, tx, tenant, "key_created", payload, time.Now()); err != nil {
				errCh <- err
				return
			}
			errCh <- tx.Commit(ctx)
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	var seqs []int64
	rows, err := pool.Query(ctx, `SELECT seq FROM audit_ledger WHERE tenant_id = $1 ORDER BY seq`, tenant)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var seq int64
		if err := rows.Scan(&seq); err != nil {
			t.Fatal(err)
		}
		seqs = append(seqs, seq)
	}
	if len(seqs) < 9 {
		t.Fatalf("seqs = %v, want the create entry plus 8 appends", seqs)
	}
	for i, seq := range seqs {
		if seq != int64(i+1) {
			t.Fatalf("seqs = %v, want no gaps", seqs)
		}
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if err := ledger.Verify(ctx, conn.Conn(), tenant); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE audit_ledger SET payload = '{"tamper":true}' WHERE tenant_id = $1 AND seq = 1`, tenant); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Verify(ctx, conn.Conn(), tenant); err == nil {
		t.Fatal("verify succeeded after a payload edit")
	}
}

func jsonBody(t *testing.T, v any) io.Reader {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(raw)
}
