package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"coldharbour/internal/ledger"
)

func TestSentinelEventsRoleAndUpsert(t *testing.T) {
	pool := phaseCDB(t)
	srv := phaseCServer(t, pool)
	defer srv.Close()
	tenant, sentinel := createTenantKey(t, pool, "sentinel")
	_, appKey := createTenantKey(t, pool, "app")

	body := map[string]any{
		"nodeId":        "host-1",
		"from":          "2026-01-01T00:00:00Z",
		"to":            "2026-01-01T00:00:10Z",
		"policyVersion": 3,
		"counts":        map[string]int{"aadhaar": 4, "email": 9},
		"held":          1,
		"dropped":       0,
		"failOpen":      0,
	}

	status, _ := postSentinelEvents(t, srv.URL, appKey, body)
	if status != http.StatusForbidden {
		t.Fatalf("app POST = %d, want 403", status)
	}

	status, raw := postSentinelEvents(t, srv.URL, sentinel, body)
	if status != http.StatusOK {
		t.Fatalf("sentinel POST = %d body %s", status, raw)
	}

	var lastSeen time.Time
	var version int
	err := pool.QueryRow(context.Background(), `
		SELECT last_seen, policy_version FROM sentinel_nodes WHERE tenant_id = $1 AND node_id = $2
	`, tenant, "host-1").Scan(&lastSeen, &version)
	if err != nil {
		t.Fatal(err)
	}
	if version != 3 {
		t.Fatalf("policy_version = %d, want 3", version)
	}
	wantTo, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:10Z")
	if !lastSeen.Equal(wantTo) {
		t.Fatalf("last_seen = %v, want %v", lastSeen, wantTo)
	}

	body["to"] = "2026-01-01T00:00:20Z"
	body["policyVersion"] = 4
	status, raw = postSentinelEvents(t, srv.URL, sentinel, body)
	if status != http.StatusOK {
		t.Fatalf("second POST = %d body %s", status, raw)
	}
	err = pool.QueryRow(context.Background(), `
		SELECT last_seen, policy_version FROM sentinel_nodes WHERE tenant_id = $1 AND node_id = $2
	`, tenant, "host-1").Scan(&lastSeen, &version)
	if err != nil {
		t.Fatal(err)
	}
	wantTo, _ = time.Parse(time.RFC3339, "2026-01-01T00:00:20Z")
	if !lastSeen.Equal(wantTo) {
		t.Fatalf("updated last_seen = %v, want %v", lastSeen, wantTo)
	}
	if version != 4 {
		t.Fatalf("updated policy_version = %d, want 4", version)
	}
}

func TestSentinelEventsLedger(t *testing.T) {
	pool := phaseCDB(t)
	srv := phaseCServer(t, pool)
	defer srv.Close()
	tenant, sentinel := createTenantKey(t, pool, "sentinel")

	status, raw := postSentinelEvents(t, srv.URL, sentinel, map[string]any{
		"nodeId":        "node-a",
		"from":          "2026-02-01T00:00:00Z",
		"to":            "2026-02-01T00:00:10Z",
		"policyVersion": 1,
		"counts":        map[string]int{"email": 2},
		"held":          0,
		"dropped":       1,
		"failOpen":      0,
	})
	if status != http.StatusOK {
		t.Fatalf("POST = %d body %s", status, raw)
	}

	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if err := ledger.Verify(ctx, conn.Conn(), tenant); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_ledger WHERE tenant_id = $1 AND kind = 'sentinel_batch'
	`, tenant).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("sentinel_batch rows = %d, want >= 1", n)
	}
}

func TestComplianceCSVBothSources(t *testing.T) {
	pool := phaseCDB(t)
	srv := phaseCServer(t, pool)
	defer srv.Close()
	_, appKey := createTenantKey(t, pool, "app")

	status, body := postJobRaw(t, srv.URL, appKey, map[string]string{
		"input":   "hello",
		"jobType": "redact",
	})
	if status != http.StatusOK {
		t.Fatalf("cold-harbour job = %d %s", status, body)
	}
	status, body = postJobRaw(t, srv.URL, appKey, map[string]string{
		"input":   "hello",
		"jobType": "redact",
		"source":  "sentinel",
	})
	if status != http.StatusOK {
		t.Fatalf("sentinel job = %d %s", status, body)
	}

	from := "2000-01-01T00:00:00Z"
	to := "2100-01-01T00:00:00Z"
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/reports/compliance?from="+from+"&to="+to, nil)
	if err != nil {
		t.Fatal(err)
	}
	authed(req, appKey)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	csv, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("compliance = %d %s", res.StatusCode, csv)
	}
	text := string(csv)
	if !bytes.Contains(csv, []byte("source")) {
		t.Fatal("CSV missing source column")
	}
	hasCH := bytes.Contains(csv, []byte(",cold-harbour\n")) || bytes.Contains(csv, []byte(",cold-harbour\r\n"))
	hasSentinel := bytes.Contains(csv, []byte(",sentinel\n")) || bytes.Contains(csv, []byte(",sentinel\r\n"))
	if !hasCH || !hasSentinel {
		t.Fatalf("CSV sources missing: cold-harbour=%v sentinel=%v\n%s", hasCH, hasSentinel, text)
	}
}

func postSentinelEvents(t *testing.T, base, key string, body map[string]any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/v1/sentinel/events", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	authed(req, key)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, b
}
