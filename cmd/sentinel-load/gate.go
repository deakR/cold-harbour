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
	minMBpsAbsolute    = 12.0
)

type Metrics struct {
	P50Ms float64 `json:"p50_ms"`
	P99Ms float64 `json:"p99_ms"`
	MBps  float64 `json:"mb_s"`
}

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

func checkGate(got Metrics, baseline Baseline) error {
	if got.P99Ms > maxP99Ms {
		return fmt.Errorf("p99 %.3f ms exceeds %.3f ms cap", got.P99Ms, maxP99Ms)
	}
	if got.MBps < minMBpsAbsolute {
		return fmt.Errorf("throughput %.3f MB/s below absolute floor %.3f MB/s", got.MBps, minMBpsAbsolute)
	}
	minMBps := baseline.MBps * minThroughputRatio
	if got.MBps < minMBps {
		return fmt.Errorf("throughput %.3f MB/s below %.0f%% of baseline %.3f MB/s (min %.3f)", got.MBps, minThroughputRatio*100, baseline.MBps, minMBps)
	}
	return nil
}
