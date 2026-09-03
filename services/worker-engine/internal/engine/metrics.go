package engine

import (
	"fmt"
	"io"
	"sync/atomic"
)

// Metrics tracks worker-engine throughput with lock-free counters.
// Exposed as Prometheus text exposition by cmd/worker via WritePrometheus.
type Metrics struct {
	Claimed   atomic.Int64
	Completed atomic.Int64
	Failed    atomic.Int64
	Recovered atomic.Int64
	Sealed    atomic.Int64
	Purged    atomic.Int64
	DLQRouted atomic.Int64
}

func (m *Metrics) IncClaimed()   { m.Claimed.Add(1) }
func (m *Metrics) IncCompleted() { m.Completed.Add(1) }
func (m *Metrics) IncFailed()    { m.Failed.Add(1) }
func (m *Metrics) IncRecovered() { m.Recovered.Add(1) }
func (m *Metrics) IncSealed()    { m.Sealed.Add(1) }
func (m *Metrics) IncPurged()    { m.Purged.Add(1) }
func (m *Metrics) IncDLQRouted() { m.DLQRouted.Add(1) }

// WritePrometheus renders counters in Prometheus text exposition format.
func (m *Metrics) WritePrometheus(w io.Writer) {
	counters := []struct {
		name  string
		help  string
		value int64
	}{
		{"coldharbor_worker_claimed_total", "Jobs claimed from the stream", m.Claimed.Load()},
		{"coldharbor_worker_completed_total", "Jobs completed, sealed, purged and acked", m.Completed.Load()},
		{"coldharbor_worker_failed_total", "Job executions that returned an error", m.Failed.Load()},
		{"coldharbor_worker_recovered_total", "PEL messages reclaimed via XAUTOCLAIM", m.Recovered.Load()},
		{"coldharbor_worker_sealed_total", "Dead drops sealed with SHA-256", m.Sealed.Load()},
		{"coldharbor_worker_purged_total", "Scratchpads purged with zero-leak verified", m.Purged.Load()},
		{"coldharbor_worker_dlq_routed_total", "Messages routed to the dead-letter stream", m.DLQRouted.Load()},
	}
	for _, c := range counters {
		fmt.Fprintf(w, "# HELP %s %s.\n# TYPE %s counter\n%s %d\n", c.name, c.help, c.name, c.name, c.value)
	}
}
