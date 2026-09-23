package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPolicyRefreshChangesOutput(t *testing.T) {
	var mu sync.Mutex
	version := 1
	detectors := `["email"]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		etag := `"1"`
		if version == 2 {
			etag = `"2"`
		}
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write([]byte(`{"version":` + itoaSmall(version) + `,"detectors":` + detectors + `,"mode":"redact","failPolicy":"closed","maxInlineBytes":65536}`))
	}))
	t.Cleanup(srv.Close)
	bin := buildSentinel(t)
	cmd := exec.Command(bin, "run", "--in", "-", "--out", "-", "--metrics", freeAddr(t), "--control-plane", srv.URL, "--policy-interval", "200ms", "--state-dir", t.TempDir())
	cmd.Env = append(os.Environ(), "SENTINEL_API_KEY=secret")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	var out safeBuf
	go func() { _, _ = io.Copy(&out, stdout) }()
	line := "jane.doe@example.com (555) 123-4567\n"
	if _, err := stdin.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	first := waitLine(t, &out, 1)
	if strings.Contains(first, "jane.doe@example.com") || !strings.Contains(first, "(555) 123-4567") {
		t.Fatal("first policy did not keep the phone and hide the email")
	}
	mu.Lock()
	version = 2
	detectors = `["email","phone_us"]`
	mu.Unlock()
	time.Sleep(700 * time.Millisecond)
	if _, err := stdin.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	second := waitLine(t, &out, 2)
	if strings.Contains(second, "(555) 123-4567") || strings.Contains(second, "jane.doe@example.com") {
		t.Fatal("updated policy did not mask the phone")
	}
}

func TestRestartUsesCachedPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"1"`)
		_, _ = w.Write([]byte(`{"version":1,"detectors":["email"],"mode":"redact","failPolicy":"closed","maxInlineBytes":65536}`))
	}))
	dir := t.TempDir()
	bin := buildSentinel(t)
	runOnce := func(url string) {
		t.Helper()
		cmd := exec.Command(bin, "run", "--in", "-", "--out", "-", "--metrics", freeAddr(t), "--control-plane", url, "--state-dir", dir, "--policy-interval", "1h")
		cmd.Env = append(os.Environ(), "SENTINEL_API_KEY=secret")
		cmd.Stdin = strings.NewReader("jane.doe@example.com\n")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("run: %v\n%s", err, out)
		}
		if strings.Contains(string(out), "jane.doe@example.com") {
			t.Fatal("policy left the email in place")
		}
	}
	runOnce(srv.URL)
	srv.Close()
	runOnce("http://127.0.0.1:1")
	cmd := exec.Command(bin, "run", "--in", "-", "--out", "-", "--metrics", freeAddr(t), "--control-plane", "http://127.0.0.1:1", "--state-dir", t.TempDir())
	cmd.Env = append(os.Environ(), "SENTINEL_API_KEY=secret")
	cmd.Stdin = strings.NewReader("jane.doe@example.com\n")
	if err := cmd.Run(); err == nil {
		t.Fatal("sentinel started with no cache and no policy file")
	}
}

func waitLine(t *testing.T, out *safeBuf, n int) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		text := out.String()
		if strings.Count(text, "\n") >= n {
			return strings.Split(text, "\n")[n-1]
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for line %d", n)
	return ""
}

func itoaSmall(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
