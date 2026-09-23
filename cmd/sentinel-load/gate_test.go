package main

import (
	"testing"
	"time"
)

func TestCheckGateLiterals(t *testing.T) {
	base := Baseline{P50Ms: 0.1, P99Ms: 1.0, MBps: 100}

	if err := checkGate(Metrics{P50Ms: 0.1, P99Ms: 6, MBps: 100}, base); err == nil {
		t.Fatal("p99 6 ms must fail the gate")
	}
	if err := checkGate(Metrics{P50Ms: 0.1, P99Ms: 1, MBps: 70}, base); err == nil {
		t.Fatal("throughput at 70% of baseline must fail the gate")
	}
	if err := checkGate(Metrics{P50Ms: 0.1, P99Ms: 1, MBps: 100}, base); err != nil {
		t.Fatalf("p99 1 ms at 100%% of baseline must pass, got %v", err)
	}
}

func TestSlowedDetectorFailsGate(t *testing.T) {
	const n = 50
	durs := make([]time.Duration, n)
	var totalBytes int64
	wallStart := time.Now()
	for i := 0; i < n; i++ {
		start := time.Now()
		time.Sleep(6 * time.Millisecond)
		durs[i] = time.Since(start)
		totalBytes += 1024
	}
	wall := time.Since(wallStart)
	got := metricsFromDurations(durs, totalBytes, wall)
	base := Baseline{P50Ms: 0.1, P99Ms: 1.0, MBps: 1000}
	if err := checkGate(got, base); err == nil {
		t.Fatalf("slowed detector sample (p99=%.3f ms) must fail the gate", got.P99Ms)
	}
}
