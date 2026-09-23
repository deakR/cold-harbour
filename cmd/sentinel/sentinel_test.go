package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNoControlPlaneImports(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "coldharbour/cmd/sentinel").CombinedOutput()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, out)
	}
	have := map[string]struct{}{}
	for _, line := range strings.Split(string(out), "\n") {
		have[strings.TrimSpace(line)] = struct{}{}
	}
	for _, banned := range []string{
		"coldharbour/internal/queue",
		"coldharbour/internal/seal",
		"coldharbour/internal/api",
		"coldharbour/internal/journal",
	} {
		if _, ok := have[banned]; ok {
			t.Fatalf("cmd/sentinel depends on %s", banned)
		}
	}
}

func TestCorpusMasksValuesAndCounts(t *testing.T) {
	bin := buildSentinel(t)
	lines, values, counts := corpusInputs(t)
	input := strings.Join(lines, "\n") + "\n"
	stdout, stderr, metrics := runBatch(t, bin, nil, input, len(lines))
	outLines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if len(outLines) != len(lines) {
		t.Fatalf("output lines = %d, want %d", len(outLines), len(lines))
	}
	if !strings.Contains(stdout, "[AADHAAR]") {
		t.Fatal("output has no [AADHAAR] token")
	}
	for i, group := range values {
		for _, value := range group {
			if strings.Contains(outLines[i], value) || strings.Contains(stderr, value) || strings.Contains(metrics, value) {
				t.Fatalf("line %d still exposes a labeled span", i)
			}
		}
	}
	for kind, want := range counts {
		got := metric(t, metrics, "sentinel_detections_total", kind)
		if got != want {
			t.Fatalf("%s detections = %g, want %g", kind, got, want)
		}
	}
	if metric(t, metrics, "sentinel_lines_total", "") != float64(len(lines)) {
		t.Fatalf("lines metric = %g, want %d", metric(t, metrics, "sentinel_lines_total", ""), len(lines))
	}
	for _, name := range []string{"sentinel_line_seconds", "sentinel_held_total", "sentinel_dropped_total", "sentinel_handoff_errors_total"} {
		if !strings.Contains(metrics, name) {
			t.Fatalf("metric %s is missing", name)
		}
	}
}

func TestShadowKeepsTheLine(t *testing.T) {
	bin := buildSentinel(t)
	in := "note jane.doe@example.com end\n"
	stdout, stderr, metrics := runBatch(t, bin, []string{"--shadow"}, in, 1)
	if stdout != in {
		t.Fatal("shadow stdout differs")
	}
	if strings.Contains(stderr, "jane.doe@example.com") || strings.Contains(metrics, "jane.doe@example.com") {
		t.Fatal("shadow path recorded the value")
	}
	if metric(t, metrics, "sentinel_detections_total", "email") != 1 {
		t.Fatalf("email detections = %g", metric(t, metrics, "sentinel_detections_total", "email"))
	}
}

func TestLongLineIsDropped(t *testing.T) {
	bin := buildSentinel(t)
	in := "jane.doe@example.com\n"
	stdout, _, metrics := runBatch(t, bin, []string{"--max-inline-bytes", "8"}, in, 1)
	if strings.TrimSpace(stdout) != "[DROPPED reason=too_large]" {
		t.Fatalf("stdout = %q", strings.TrimSpace(stdout))
	}
	if metric(t, metrics, "sentinel_dropped_total", "") != 1 {
		t.Fatalf("dropped = %g", metric(t, metrics, "sentinel_dropped_total", ""))
	}
}

func TestFollowMasksAppendedLine(t *testing.T) {
	bin := buildSentinel(t)
	dir := t.TempDir()
	inPath := filepath.Join(dir, "app.log")
	outPath := filepath.Join(dir, "masked.log")
	if err := os.WriteFile(inPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	addr := freeAddr(t)
	cmd := exec.Command(bin, "run", "--in", inPath, "--follow", "--out", outPath, "--metrics", addr)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	f, err := os.OpenFile(inPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("note jane.doe@example.com end\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		body, _ := os.ReadFile(outPath)
		text := string(body)
		if strings.Contains(text, "[EMAIL]") && !strings.Contains(text, "jane.doe@example.com") {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("followed line was not masked")
}

func buildSentinel(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "sentinel"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", bin, "coldharbour/cmd/sentinel")
	cmd.Dir = moduleDir(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func moduleDir(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "coldharbour").CombinedOutput()
	if err != nil {
		t.Fatalf("module dir: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

type safeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *safeBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *safeBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func runBatch(t *testing.T, bin string, extra []string, input string, wantLines int) (string, string, string) {
	t.Helper()
	addr := freeAddr(t)
	args := append([]string{"run", "--metrics", addr, "--in", "-", "--out", "-"}, extra...)
	cmd := exec.Command(bin, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr safeBuf
	var out safeBuf
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = io.Copy(&out, stdout)
	}()
	go func() {
		_, _ = io.WriteString(stdin, input)
	}()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) && strings.Count(out.String(), "\n") < wantLines {
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Count(out.String(), "\n") < wantLines {
		_ = cmd.Process.Kill()
		t.Fatalf("timed out with %d lines", strings.Count(out.String(), "\n"))
	}
	metrics := scrape(t, addr)
	_ = stdin.Close()
	_ = cmd.Wait()
	return out.String(), stderr.String(), metrics
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func scrape(t *testing.T, addr string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		res, err := http.Get("http://" + addr + "/metrics")
		if err != nil {
			last = err
			time.Sleep(20 * time.Millisecond)
			continue
		}
		body, err := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	t.Fatalf("metrics: %v", last)
	return ""
}

func metric(t *testing.T, body, name, kind string) float64 {
	t.Helper()
	prefix := name + " "
	if kind != "" {
		prefix = name + `{kind="` + kind + `"} `
	}
	for _, ln := range strings.Split(body, "\n") {
		if strings.HasPrefix(ln, prefix) {
			fields := strings.Fields(ln)
			v, err := strconv.ParseFloat(fields[len(fields)-1], 64)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
	}
	if kind == "" {
		t.Fatalf("metric %s missing", name)
	}
	return 0
}

func corpusInputs(t *testing.T) ([]string, [][]string, map[string]float64) {
	t.Helper()
	f, err := os.Open(filepath.Join(moduleDir(t), "internal", "detect", "testdata", "corpus.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines []string
	var values [][]string
	counts := map[string]float64{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var row struct {
			Text  string `json:"text"`
			Spans []struct {
				Kind  string `json:"kind"`
				Start int    `json:"start"`
				End   int    `json:"end"`
			} `json:"spans"`
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, row.Text)
		var group []string
		for _, sp := range row.Spans {
			group = append(group, row.Text[sp.Start:sp.End])
			counts[sp.Kind]++
		}
		values = append(values, group)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return lines, values, counts
}
