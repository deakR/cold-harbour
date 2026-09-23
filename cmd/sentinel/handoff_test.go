package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFailPolicyWhenHandoffDown(t *testing.T) {
	bin := buildSentinel(t)
	secret := "jane.doe@example.com"
	long := secret + strings.Repeat("x", 32)

	t.Run("closed", func(t *testing.T) {
		policy := filepath.Join(t.TempDir(), "policy.json")
		writePolicyFile(t, policy, `{"version":1,"detectors":["email"],"mode":"redact","failPolicy":"closed","maxInlineBytes":8}`)
		stdout, stderr, _ := runBatch(t, bin, []string{
			"--max-inline-bytes", "8",
			"--policy", policy,
			"--control-plane", "http://127.0.0.1:1",
		}, long+"\n", 1)
		got := strings.TrimSpace(stdout)
		if got != "[DROPPED reason=handoff_unavailable]" {
			t.Fatalf("stdout = %q", got)
		}
		if strings.Contains(stdout, secret) || strings.Contains(stderr, secret) {
			t.Fatal("closed path exposed the raw value")
		}
	})

	t.Run("inline", func(t *testing.T) {
		policy := filepath.Join(t.TempDir(), "policy.json")
		writePolicyFile(t, policy, `{"version":1,"detectors":["email"],"mode":"redact","failPolicy":"inline","maxInlineBytes":8}`)
		stdout, stderr, _ := runBatch(t, bin, []string{
			"--max-inline-bytes", "8",
			"--policy", policy,
			"--control-plane", "http://127.0.0.1:1",
		}, long+"\n", 1)
		got := strings.TrimSpace(stdout)
		if !strings.Contains(got, "[EMAIL]") {
			t.Fatalf("inline stdout = %q", got)
		}
		if strings.Contains(got, secret) || strings.Contains(stderr, secret) {
			t.Fatal("inline path exposed the raw value")
		}
	})

	t.Run("open", func(t *testing.T) {
		policy := filepath.Join(t.TempDir(), "policy.json")
		writePolicyFile(t, policy, `{"version":1,"detectors":["email"],"mode":"redact","failPolicy":"open","maxInlineBytes":8}`)
		stdout, stderr, _ := runBatch(t, bin, []string{
			"--max-inline-bytes", "8",
			"--policy", policy,
			"--control-plane", "http://127.0.0.1:1",
			"--allow-fail-open",
		}, long+"\n", 1)
		got := strings.TrimSpace(stdout)
		if got != long {
			t.Fatalf("open stdout = %q", got)
		}
		if strings.Contains(stderr, secret) {
			t.Fatal("open path logged the raw value")
		}
	})

	t.Run("open without flag is closed", func(t *testing.T) {
		policy := filepath.Join(t.TempDir(), "policy.json")
		writePolicyFile(t, policy, `{"version":1,"detectors":["email"],"mode":"redact","failPolicy":"open","maxInlineBytes":8}`)
		stdout, _, _ := runBatch(t, bin, []string{
			"--max-inline-bytes", "8",
			"--policy", policy,
			"--control-plane", "http://127.0.0.1:1",
		}, long+"\n", 1)
		if strings.TrimSpace(stdout) != "[DROPPED reason=handoff_unavailable]" {
			t.Fatalf("stdout = %q", strings.TrimSpace(stdout))
		}
	})
}

func TestFullQueueOrBreakerDoesNotBlockReader(t *testing.T) {
	bin := buildSentinel(t)
	var started atomic.Int64
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/policy" {
			w.Header().Set("ETag", `"1"`)
			_, _ = w.Write([]byte(`{"version":1,"detectors":["email"],"mode":"redact","failPolicy":"closed","maxInlineBytes":8}`))
			return
		}
		if r.URL.Path == "/v1/jobs" && r.Method == http.MethodPost {
			started.Add(1)
			<-block
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(func() {
		close(block)
		srv.Close()
	})

	long := "jane.doe@example.com" + strings.Repeat("y", 40)
	var input strings.Builder
	for i := 0; i < 1002; i++ {
		input.WriteString(long)
		input.WriteByte('\n')
	}
	input.WriteString("short ok\n")

	addr := freeAddr(t)
	cmd := exec.Command(bin, "run",
		"--in", "-", "--out", "-",
		"--metrics", addr,
		"--control-plane", srv.URL,
		"--state-dir", t.TempDir(),
		"--policy-interval", "1h",
		"--max-inline-bytes", "8",
	)
	cmd.Env = append(os.Environ(), "SENTINEL_API_KEY=secret")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var out safeBuf
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
	go func() { _, _ = io.WriteString(stdin, input.String()) }()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		text := out.String()
		if strings.Contains(text, "short ok") {
			if !strings.Contains(text, "[DROPPED reason=handoff_unavailable]") {
				t.Fatal("full queue did not apply closed fail policy")
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("reader blocked; started posts=%d", started.Load())
}

func TestBreakerProbeCloses(t *testing.T) {
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/jobs" {
			http.NotFound(w, r)
			return
		}
		if posts.Add(1) <= 5 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jobId":"job-recovered","status":"QUEUED"}`))
	}))
	t.Cleanup(srv.Close)
	bin := buildSentinel(t)
	policy := filepath.Join(t.TempDir(), "policy.json")
	writePolicyFile(t, policy, `{"version":1,"detectors":["email"],"mode":"redact","failPolicy":"closed","maxInlineBytes":8}`)
	long := "jane.doe@example.com" + strings.Repeat("z", 40)
	stdout, stderr, _ := runBatch(t, bin, []string{
		"--max-inline-bytes", "8",
		"--policy", policy,
		"--control-plane", srv.URL,
		"--policy-interval", "1h",
	}, long+"\n"+long+"\n", 2)
	if strings.Contains(stderr, "jane.doe@example.com") {
		t.Fatal("breaker probe logged the raw value")
	}
	if !strings.Contains(stdout, "[DROPPED reason=handoff_unavailable]") {
		t.Fatal("first long line was not dropped after the breaker opened")
	}
	if !strings.Contains(stdout, "[HELD job=job-recovered]") {
		t.Fatal("breaker stayed open after Cold Harbour recovered")
	}
}

func writePolicyFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
