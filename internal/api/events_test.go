package api

import (
	"context"
	"net/http"
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

func TestWebSocketRejectsForeignOrigin(t *testing.T) {
	pool := phaseCDB(t)
	_, key := createTenantKey(t, pool, "app")
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	apiSrv := New(pool, rdb, seal.NewMemoryKeyStore(), queue.OpenJobs(mr.Addr()), []string{"redact"})
	apiSrv.SetWSOrigins([]string{"localhost:5173"})
	srv := httptest.NewServer(apiSrv.Handler())
	t.Cleanup(srv.Close)

	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/ws/events?apiKey=" + url.QueryEscape(key)
	ctx := context.Background()
	_, resp, err := websocket.Dial(ctx, u, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"https://evil.example"}},
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

	conn, _, err := websocket.Dial(ctx, u, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://localhost:5173"}},
	})
	if err != nil {
		t.Fatalf("dashboard origin: %v", err)
	}
	_ = conn.Close(websocket.StatusNormalClosure, "")
}
