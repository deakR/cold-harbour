package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	lineCount   = 100_000
	lineBytes   = 1024
	baselineRel = "bench/baseline.json"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sentinel-load: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	lines := make([]string, lineCount)
	var totalBytes int64
	for i := 0; i < lineCount; i++ {
		lines[i] = makeLine(i)
		totalBytes += int64(len(lines[i]))
	}

	durs := make([]time.Duration, lineCount)
	wallStart := time.Now()
	for i, line := range lines {
		start := time.Now()
		_ = maskLine(line)
		durs[i] = time.Since(start)
	}
	wall := time.Since(wallStart)

	got := metricsFromDurations(durs, totalBytes, wall)
	fmt.Printf("p50_ms=%.6f p99_ms=%.6f mb_s=%.6f\n", got.P50Ms, got.P99Ms, got.MBps)

	path := baselineRel
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		out, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			return err
		}
		out = append(out, '\n')
		if err := os.WriteFile(path, out, 0o600); err != nil {
			return err
		}
		fmt.Printf("wrote baseline %s\n", path)
		return nil
	}

	var baseline Baseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return fmt.Errorf("parse baseline: %w", err)
	}
	return checkGate(got, baseline)
}

func makeLine(i int) string {
	var body string
	if i%20 == 0 {
		body = fmt.Sprintf("user%d@example.com ", i)
	} else {
		body = "pad "
	}
	if len(body) >= lineBytes {
		return body[:lineBytes]
	}
	return body + strings.Repeat("x", lineBytes-len(body))
}
