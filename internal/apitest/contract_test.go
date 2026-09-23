package apitest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const (
	keyA = "ch_live_a_demo_key_aaaaaaaa"
	keyB = "ch_live_b_demo_key_bbbbbbbb"
)

func baseURL(t *testing.T) string {
	t.Helper()
	base := os.Getenv("API_BASE")
	if base == "" {
		t.Skip("set API_BASE to run contract tests")
	}
	return strings.TrimRight(base, "/")
}

func apiPath(p string) string {
	if strings.HasPrefix(p, "/d/") {
		return p
	}
	return os.Getenv("API_PREFIX") + p
}

func call(t *testing.T, method, path, key string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, baseURL(t)+apiPath(path), r)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, got
}

func postJob(t *testing.T, key, input string) string {
	t.Helper()
	status, body := call(t, http.MethodPost, "/jobs", key, map[string]string{
		"input":   input,
		"jobType": "redact",
	})
	if status != http.StatusOK {
		t.Fatalf("POST /jobs = %d, body %s", status, body)
	}
	var payload struct {
		JobID  string `json:"jobId"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.JobID == "" || payload.Status != "QUEUED" {
		t.Fatalf("POST /jobs body = %s", body)
	}
	return payload.JobID
}

func waitTerminal(t *testing.T, key, jobID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var lastStatus int
	var lastBody []byte
	for time.Now().Before(deadline) {
		status, body := call(t, http.MethodGet, "/jobs/"+jobID, key, nil)
		lastStatus, lastBody = status, body
		if status == http.StatusOK {
			var payload map[string]any
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			switch payload["status"] {
			case "COMPLETED", "FAILED":
				return payload
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish, last GET = %d %s", jobID, lastStatus, lastBody)
	return nil
}

func assertField400(t *testing.T, status int, body []byte, field string) {
	t.Helper()
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", status, body)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("body %s: %v", body, err)
	}
	if payload["field"] != field {
		t.Fatalf("field = %v, want %s, body %s", payload["field"], field, body)
	}
}

func future(d time.Duration) string {
	return time.Now().UTC().Add(d).Format(time.RFC3339)
}

func withWorkerPaused(t *testing.T) {
	t.Helper()
	name := os.Getenv("COLDHARBOUR_WORKER_CONTAINER")
	if name == "" {
		t.Fatal("COLDHARBOUR_WORKER_CONTAINER is required so the job stays queued")
	}
	out, err := exec.Command("docker", "pause", name).CombinedOutput()
	if err != nil {
		t.Fatalf("docker pause %s: %v %s", name, err, out)
	}
	t.Cleanup(func() {
		out, err := exec.Command("docker", "unpause", name).CombinedOutput()
		if err != nil {
			t.Errorf("docker unpause %s: %v %s", name, err, out)
		}
	})
}

func TestUnauthorized(t *testing.T) {
	status, _ := call(t, http.MethodGet, "/jobs", "", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("missing key status = %d, want 401", status)
	}
	status, _ = call(t, http.MethodGet, "/jobs", "not-a-key", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("bad key status = %d, want 401", status)
	}
}

func TestMissingInputAndUnknownTypeAre400(t *testing.T) {
	status, body := call(t, http.MethodPost, "/jobs", keyA, map[string]string{})
	if status != http.StatusBadRequest {
		t.Fatalf("missing input status = %d, body %s", status, body)
	}
	status, body = call(t, http.MethodPost, "/jobs", keyA, map[string]string{
		"input":   "hello",
		"jobType": "no-such-type",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("unknown jobType status = %d, body %s", status, body)
	}
}

func TestCrossTenantJobIs404(t *testing.T) {
	id := postJob(t, keyA, "Email a@b.com from tenant a")
	status, body := call(t, http.MethodGet, "/jobs/"+id, keyB, nil)
	if status != http.StatusNotFound {
		t.Fatalf("cross-tenant GET = %d, body %s", status, body)
	}
	status, body = call(t, http.MethodPost, "/jobs/"+id+"/delivery-links", keyB, map[string]string{
		"expiresAt": future(time.Hour),
		"maxViews":  "1",
	})
	if status != http.StatusNotFound {
		t.Fatalf("cross-tenant delivery = %d, body %s", status, body)
	}
	status, _ = call(t, http.MethodGet, "/jobs/does-not-exist", keyA, nil)
	if status != http.StatusNotFound {
		t.Fatalf("missing job status = %d, want 404", status)
	}
}

func TestPostListAndFetch(t *testing.T) {
	id := postJob(t, keyA, "Email a@b.com about the report")
	status, body := call(t, http.MethodGet, "/jobs", keyA, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /jobs = %d, body %s", status, body)
	}
	if !bytes.Contains(body, []byte(id)) {
		t.Fatalf("list %s does not contain %s", body, id)
	}
	got := waitTerminal(t, keyA, id)
	if got["status"] != "COMPLETED" {
		t.Fatalf("status = %v, want COMPLETED", got["status"])
	}
	if got["tenantId"] != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("tenantId = %v", got["tenantId"])
	}
}

func TestReportCSVHeader(t *testing.T) {
	q := url.Values{}
	q.Set("from", "2000-01-01T00:00:00Z")
	q.Set("to", "2100-01-01T00:00:00Z")
	status, body := call(t, http.MethodGet, "/reports/compliance?"+q.Encode(), keyA, nil)
	if status != http.StatusOK {
		t.Fatalf("report status = %d, body %s", status, body)
	}
	first, _, _ := strings.Cut(string(body), "\n")
	const header = "dispatch_time,completion_time,final_state,checksum,signature_status,purge_timestamp"
	if first != header {
		t.Fatalf("header = %q, want %q", first, header)
	}
}

func TestReportBadDateIs400(t *testing.T) {
	q := url.Values{}
	q.Set("from", "yesterday")
	q.Set("to", "2100-01-01T00:00:00Z")
	status, body := call(t, http.MethodGet, "/reports/compliance?"+q.Encode(), keyA, nil)
	assertField400(t, status, body, "from")
}

func TestDeliveryBadValuesAre400(t *testing.T) {
	id := postJob(t, keyA, "Email a@b.com delivery validation")
	waitTerminal(t, keyA, id)
	cases := []struct {
		name  string
		body  map[string]string
		field string
	}{
		{"not a time", map[string]string{"expiresAt": "tomorrow", "maxViews": "1"}, "expiresAt"},
		{"past", map[string]string{"expiresAt": "2000-01-01T00:00:00Z", "maxViews": "1"}, "expiresAt"},
		{"too far", map[string]string{"expiresAt": future(31 * 24 * time.Hour), "maxViews": "1"}, "expiresAt"},
		{"not a number", map[string]string{"expiresAt": future(time.Hour), "maxViews": "many"}, "maxViews"},
		{"too high", map[string]string{"expiresAt": future(time.Hour), "maxViews": "101"}, "maxViews"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := call(t, http.MethodPost, "/jobs/"+id+"/delivery-links", keyA, tc.body)
			assertField400(t, status, body, tc.field)
		})
	}
}

func TestDeliveryMaxViewsBelowOneIs400(t *testing.T) {
	id := postJob(t, keyA, "Email a@b.com max views")
	waitTerminal(t, keyA, id)
	status, body := call(t, http.MethodPost, "/jobs/"+id+"/delivery-links", keyA, map[string]string{
		"expiresAt": future(time.Hour),
		"maxViews":  "0",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body %s", status, body)
	}
}

func TestDeliveryExpiredIs410(t *testing.T) {
	id := postJob(t, keyA, "Email a@b.com expiry")
	waitTerminal(t, keyA, id)
	status, body := call(t, http.MethodPost, "/jobs/"+id+"/delivery-links", keyA, map[string]string{
		"expiresAt": future(2 * time.Second),
		"maxViews":  "2",
	})
	if status != http.StatusOK {
		t.Fatalf("create = %d, body %s", status, body)
	}
	var link struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &link); err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Second)
	status, body = call(t, http.MethodGet, "/d/"+link.Token, "", nil)
	if status != http.StatusGone {
		t.Fatalf("open expired = %d, want 410, body %s", status, body)
	}
}

func TestDeliveryViewLimitIs403(t *testing.T) {
	id := postJob(t, keyA, "Email a@b.com views")
	waitTerminal(t, keyA, id)
	status, body := call(t, http.MethodPost, "/jobs/"+id+"/delivery-links", keyA, map[string]string{
		"expiresAt": future(time.Hour),
		"maxViews":  "1",
	})
	if status != http.StatusOK {
		t.Fatalf("create = %d, body %s", status, body)
	}
	var link struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &link); err != nil {
		t.Fatal(err)
	}
	status, body = call(t, http.MethodGet, "/d/"+link.Token, "", nil)
	if status != http.StatusOK {
		t.Fatalf("first open = %d, body %s", status, body)
	}
	status, body = call(t, http.MethodGet, "/d/"+link.Token, "", nil)
	if status != http.StatusForbidden {
		t.Fatalf("second open = %d, want 403, body %s", status, body)
	}
}

func TestDeliveryBeforeOutputIs409(t *testing.T) {
	baseURL(t)
	withWorkerPaused(t)
	id := postJob(t, keyA, "Email a@b.com still queued")
	status, body := call(t, http.MethodGet, "/jobs/"+id, keyA, nil)
	if status != http.StatusOK || !bytes.Contains(body, []byte(`"QUEUED"`)) {
		t.Fatalf("queued job status = %d body %s", status, body)
	}
	status, body = call(t, http.MethodPost, "/jobs/"+id+"/delivery-links", keyA, map[string]string{
		"expiresAt": future(time.Hour),
		"maxViews":  "1",
	})
	if status != http.StatusConflict {
		t.Fatalf("create before output = %d, want 409, body %s", status, body)
	}
}

func TestWebSocketFiltersByTenant(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dial := func(key string) *websocket.Conn {
		t.Helper()
		ws := strings.Replace(baseURL(t), "http://", "ws://", 1)
		ws = strings.Replace(ws, "https://", "wss://", 1)
		u := ws + apiPath("/ws/events") + "?apiKey=" + url.QueryEscape(key)
		conn, _, err := websocket.Dial(ctx, u, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "") })
		return conn
	}
	connA := dial(keyA)
	connB := dial(keyB)
	id := postJob(t, keyA, "Email a@b.com over the socket")
	readUntil := func(conn *websocket.Conn, wait time.Duration) string {
		t.Helper()
		cctx, stop := context.WithTimeout(ctx, wait)
		defer stop()
		_, msg, err := conn.Read(cctx)
		if err != nil {
			return ""
		}
		return string(msg)
	}
	deadline := time.Now().Add(15 * time.Second)
	saw := ""
	for time.Now().Before(deadline) && !strings.Contains(saw, id) {
		saw += readUntil(connA, time.Second)
	}
	if !strings.Contains(saw, id) {
		t.Fatalf("tenant A socket did not receive %s", id)
	}
	other := readUntil(connB, time.Second)
	if strings.Contains(other, id) {
		t.Fatalf("tenant B socket received %s", other)
	}
}

func TestPostRateLimitIs429(t *testing.T) {
	saw := false
	for i := 0; i < 40; i++ {
		status, body := call(t, http.MethodPost, "/jobs", keyA, map[string]string{
			"input":   "rate limit probe",
			"jobType": "redact",
		})
		if status == http.StatusTooManyRequests {
			saw = true
			break
		}
		if status != http.StatusOK {
			t.Fatalf("POST %d = %d, body %s", i, status, body)
		}
	}
	if !saw {
		t.Fatal("no 429 after 40 posts")
	}
}
