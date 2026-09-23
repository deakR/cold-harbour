package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"coldharbour/internal/keys"
	"coldharbour/internal/queue"
	"coldharbour/internal/seal"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

func sessionServer(t *testing.T, pool *pgxpool.Pool) (*Server, *httptest.Server) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	apiSrv := New(pool, rdb, seal.NewMemoryKeyStore(), queue.OpenJobs(mr.Addr()), []string{"redact", "mask"})
	apiSrv.SetWSOrigins([]string{"localhost:5173"})
	apiSrv.SetInsecureCookies(true)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)
	return apiSrv, srv
}

func jarClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar}
}

func createTenantKeyID(t *testing.T, pool *pgxpool.Pool, role string) (tenant, keyID uuid.UUID, full string) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `INSERT INTO tenants (name) VALUES ($1) RETURNING id`, "t-"+role+"-"+uuid.NewString()).Scan(&tenant); err != nil {
		t.Fatal(err)
	}
	keyID, _, full, err = keys.Create(ctx, tx, tenant, role, role)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return tenant, keyID, full
}

func postLogin(t *testing.T, client *http.Client, base, apiKey string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/v1/session/login", jsonBody(t, map[string]string{"apiKey": apiKey}))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func loginOK(t *testing.T, client *http.Client, base, apiKey string) string {
	t.Helper()
	res := postLogin(t, client, base, apiKey)
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("login = %d %s", res.StatusCode, b)
	}
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.CSRFToken == "" {
		t.Fatal("login missing csrfToken")
	}
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	cookies := client.Jar.Cookies(u)
	if len(cookies) == 0 {
		t.Fatal("login set no cookie")
	}
	return body.CSRFToken
}

func TestAdminLoginCookieGetsJobs(t *testing.T) {
	pool := phaseCDB(t)
	_, srv := sessionServer(t, pool)
	_, _, admin := createTenantKeyID(t, pool, "admin")
	client := jarClient(t)
	_ = loginOK(t, client, srv.URL, admin)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("GET /v1/jobs with cookie = %d %s", res.StatusCode, b)
	}
}

func TestSentinelLoginForbidden(t *testing.T) {
	pool := phaseCDB(t)
	_, srv := sessionServer(t, pool)
	_, _, sentinel := createTenantKeyID(t, pool, "sentinel")
	client := jarClient(t)
	res := postLogin(t, client, srv.URL, sentinel)
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("sentinel login = %d, want 403", res.StatusCode)
	}
}

func TestIdleAbsoluteRevokedSessionUnauthorized(t *testing.T) {
	pool := phaseCDB(t)
	_, srv := sessionServer(t, pool)
	tenant, _, admin := createTenantKeyID(t, pool, "admin")
	client := jarClient(t)
	_ = loginOK(t, client, srv.URL, admin)

	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		UPDATE sessions SET last_seen_at = now() - interval '31 minutes'
		WHERE tenant_id = $1 AND revoked_at IS NULL
	`, tenant); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("idle session = %d, want 401", res.StatusCode)
	}

	client = jarClient(t)
	_ = loginOK(t, client, srv.URL, admin)
	if _, err := pool.Exec(ctx, `
		UPDATE sessions SET expires_at = now() - interval '1 second', last_seen_at = now()
		WHERE tenant_id = $1 AND revoked_at IS NULL
	`, tenant); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("absolute session = %d, want 401", res.StatusCode)
	}

	client = jarClient(t)
	_ = loginOK(t, client, srv.URL, admin)
	if _, err := pool.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE tenant_id = $1 AND revoked_at IS NULL
	`, tenant); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked session = %d, want 401", res.StatusCode)
	}
}

func TestSessionWithRevokedAPIKeyUnauthorized(t *testing.T) {
	pool := phaseCDB(t)
	_, srv := sessionServer(t, pool)
	tenant, keyID, admin := createTenantKeyID(t, pool, "admin")
	client := jarClient(t)
	_ = loginOK(t, client, srv.URL, admin)

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := keys.Revoke(ctx, tx, tenant, keyID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/jobs", nil)
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked key session = %d, want 401", res.StatusCode)
	}
}

func TestWebSocketCookieAndRejectsQueryAPIKey(t *testing.T) {
	pool := phaseCDB(t)
	_, srv := sessionServer(t, pool)
	_, _, admin := createTenantKeyID(t, pool, "admin")
	client := jarClient(t)
	_ = loginOK(t, client, srv.URL, admin)

	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	cookies := client.Jar.Cookies(base)
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}
	cookieHdr := cookies[0].Name + "=" + cookies[0].Value
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/ws/events"
	ctx := context.Background()

	_, resp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"https://evil.example"},
			"Cookie": []string{cookieHdr},
		},
	})
	if err == nil {
		t.Fatal("foreign origin connected")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("foreign origin status = %d, want 403", code)
	}

	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{"http://localhost:5173"},
			"Cookie": []string{cookieHdr},
		},
	})
	if err != nil {
		t.Fatalf("cookie websocket: %v", err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")

	queryURL := wsURL + "?apiKey=" + url.QueryEscape(admin)
	_, resp, err = websocket.Dial(ctx, queryURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://localhost:5173"}},
	})
	if err == nil {
		t.Fatal("query apiKey connected")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		code := 0
		if resp != nil {
			code = resp.StatusCode
		}
		t.Fatalf("query apiKey status = %d, want 401", code)
	}
}

func TestSentinelAPIKeyStillPostsEvents(t *testing.T) {
	pool := phaseCDB(t)
	_, srv := sessionServer(t, pool)
	_, _, sentinel := createTenantKeyID(t, pool, "sentinel")
	body := map[string]any{
		"nodeId":        "n1",
		"from":          time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
		"to":            time.Now().UTC().Format(time.RFC3339Nano),
		"policyVersion": 1,
		"counts":        map[string]int{"email": 1},
		"held":          1,
		"dropped":       0,
		"failOpen":      0,
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/sentinel/events", jsonBody(t, body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	authed(req, sentinel)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("sentinel events = %d %s", res.StatusCode, b)
	}
}

func TestCookiePOSTWithoutCSRFForbidden(t *testing.T) {
	pool := phaseCDB(t)
	_, srv := sessionServer(t, pool)
	_, _, admin := createTenantKeyID(t, pool, "admin")
	client := jarClient(t)
	_ = loginOK(t, client, srv.URL, admin)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/jobs", jsonBody(t, map[string]string{
		"input":   "x",
		"jobType": "redact",
	}))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:5173")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("cookie POST without CSRF = %d %s", res.StatusCode, b)
	}
}
