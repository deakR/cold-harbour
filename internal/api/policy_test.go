package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"coldharbour/internal/keys"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPolicyPutGetETagAndLedger(t *testing.T) {
	pool := phaseCDB(t)
	srv := phaseCServer(t, pool)
	defer srv.Close()
	tenant, admin := createTenantKey(t, pool, "admin")
	sentinelKey := keyOnTenant(t, pool, tenant, "sentinel")

	body := map[string]any{
		"detectors":      []string{"email", "aadhaar"},
		"mode":           "redact",
		"failPolicy":     "closed",
		"maxInlineBytes": 65536,
	}
	status, raw, etag := doPolicy(t, srv.URL, http.MethodPut, sentinelKey, body, "")
	if status != http.StatusForbidden {
		t.Fatalf("sentinel PUT = %d, want 403", status)
	}

	status, raw, etag = doPolicy(t, srv.URL, http.MethodPut, admin, body, "")
	if status != http.StatusOK {
		t.Fatalf("PUT = %d %s", status, raw)
	}
	var stored struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Version != 1 {
		t.Fatalf("version = %d, want 1", stored.Version)
	}
	if etag == "" {
		t.Fatal("missing ETag")
	}

	status, _, _ = doPolicy(t, srv.URL, http.MethodGet, sentinelKey, nil, etag)
	if status != http.StatusNotModified {
		t.Fatalf("GET = %d, want 304", status)
	}

	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_ledger WHERE tenant_id = $1 AND kind = 'policy_changed'`, tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("policy_changed rows = %d, want 1", n)
	}
}

func keyOnTenant(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID, role string) string {
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

func doPolicy(t *testing.T, base, method, key string, body any, etag string) (int, []byte, string) {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = jsonBody(t, body)
	}
	req, err := http.NewRequest(method, base+"/v1/policy", rdr)
	if err != nil {
		t.Fatal(err)
	}
	authed(req, key)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	return res.StatusCode, raw, res.Header.Get("ETag")
}
