package api

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"coldharbour/internal/queue"
	"coldharbour/internal/seal"

	"github.com/alicebob/miniredis/v2"
	"github.com/coder/websocket"
	"github.com/redis/go-redis/v9"
)

func TestParseWSOrigins(t *testing.T) {
	got := ParseWSOrigins(" dash.example , localhost:4173 ")
	if len(got) != 2 || got[0] != "dash.example" || got[1] != "localhost:4173" {
		t.Fatalf("parsed = %#v", got)
	}
	got = ParseWSOrigins("")
	if len(got) != 2 || got[0] != "localhost:5173" || got[1] != "127.0.0.1:5173" {
		t.Fatalf("default = %#v", got)
	}
}

func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	pool := phaseCDB(t)
	_, key := createTenantKey(t, pool, "app")
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	apiSrv := New(pool, rdb, seal.NewMemoryKeyStore(), queue.OpenJobs(mr.Addr()), []string{"redact"})
	apiSrv.SetWSOrigins([]string{"localhost:5173"})
	apiSrv.SetInsecureCookies(true)
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	_ = loginOK(t, client, srv.URL, key)

	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	cookies := jar.Cookies(base)
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
		t.Fatalf("dashboard origin: %v", err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
}
