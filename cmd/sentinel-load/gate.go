package main

import (
	"fmt"
	"math"
	"sort"
	"time"
)

const (
	maxP99Ms           = 5.0
	minThroughputRatio = 0.80
)

// Metrics holds one load-run sample for the latency and throughput gate.
type Metrics struct {
	P50Ms float64 `json:"p50_ms"`
	P99Ms float64 `json:"p99_ms"`
	MBps  float64 `json:"mb_s"`
}

// Baseline is the committed reference Metrics in bench/baseline.json.
type Baseline = Metrics

func metricsFromDurations(durs []time.Duration, totalBytes int64, wall time.Duration) Metrics {
	sorted := append([]time.Duration(nil), durs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p50 := percentile(sorted, 0.50)
	p99 := percentile(sorted, 0.99)
	sec := wall.Seconds()
	var mbps float64
	if sec > 0 {
		mbps = float64(totalBytes) / (1024 * 1024) / sec
	}
	return Metrics{
		P50Ms: float64(p50) / float64(time.Millisecond),
		P99Ms: float64(p99) / float64(time.Millisecond),
		MBps:  mbps,
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// checkGate is intentionally wrong in the first commit so the proofs fail red.
func checkGate(got Metrics, baseline Baseline) error {
	_ = got
	_ = baseline
	_ = maxP99Ms
	_ = minThroughputRatio
	_ = fmt.Errorf
	return nil
}
