package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"coldharbour/internal/detect"
)

func TestFlushRestoresWindowWhenPostFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	active.Store(maskSettings{maxInline: 65536, kinds: maskKinds, mode: detect.ModeRedact, version: 1})
	e := &eventsLoop{
		baseURL: srv.URL,
		apiKey:  "k",
		nodeID:  "n",
		client:  &http.Client{Timeout: time.Second},
		window:  newWindowCounters(time.Now().UTC()),
	}
	e.record(maskResult{counts: map[detect.Kind]int{detect.KindEmail: 2}, dropped: true})
	e.flush()
	e.window.mu.Lock()
	defer e.window.mu.Unlock()
	if e.window.counts[detect.KindEmail] != 2 || e.window.dropped != 1 {
		t.Fatal("failed post dropped the window")
	}
}

func TestEventsBatchPosted(t *testing.T) {
	bin := buildSentinel(t)
	var mu sync.Mutex
	var posts []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/policy" && r.Method == http.MethodGet:
			w.Header().Set("ETag", `"1"`)
			_, _ = w.Write([]byte(`{"version":1,"detectors":["email","aadhaar"],"mode":"redact","failPolicy":"closed","maxInlineBytes":65536}`))
		case r.URL.Path == "/v1/sentinel/events" && r.Method == http.MethodPost:
			if r.Header.Get("X-API-Key") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var body map[string]any
			if err := json.Unmarshal(raw, &body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			posts = append(posts, body)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	secret := "jane.doe@example.com"
	input := "note " + secret + " end\n"
	addr := freeAddr(t)
	cmd := exec.Command(bin, "run",
		"--in", "-", "--out", "-",
		"--metrics", addr,
		"--control-plane", srv.URL,
		"--events-interval", "200ms",
		"--node-id", "test-node",
	)
	cmd.Env = append(os.Environ(), "SENTINEL_API_KEY=test-sentinel-key")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr safeBuf
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stdin, input); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("sentinel exit: %v stderr=%s", err, stderr.String())
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("sentinel timed out")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(posts)
		mu.Unlock()
		if n >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(posts) < 1 {
		t.Fatalf("no events POST; stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	body := posts[0]
	for _, key := range []string{"failOpen", "held", "dropped", "counts", "nodeId", "from", "to", "policyVersion"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing %s in %v", key, body)
		}
	}
	if body["nodeId"] != "test-node" {
		t.Fatalf("nodeId = %v", body["nodeId"])
	}
	counts, ok := body["counts"].(map[string]any)
	if !ok {
		t.Fatalf("counts type %T", body["counts"])
	}
	if email, _ := counts["email"].(float64); email < 1 {
		t.Fatalf("counts.email = %v, want >= 1", counts["email"])
	}
	rawLine := secret
	for k, v := range body {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if k == "nodeId" || k == "from" || k == "to" {
			continue
		}
		if strings.Contains(s, rawLine) {
			t.Fatalf("string field %s contains raw line text", k)
		}
	}
	for _, v := range counts {
		if s, ok := v.(string); ok && strings.Contains(s, rawLine) {
			t.Fatalf("counts value contains raw line text: %s", s)
		}
	}
	if strings.Contains(stderr.String(), "test-sentinel-key") {
		t.Fatal("stderr logged the API key")
	}
	if strings.Contains(stdout.String(), rawLine) {
		t.Fatal("stdout kept the raw value")
	}
}
