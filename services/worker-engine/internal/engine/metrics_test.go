package engine

import (
	"strings"
	"testing"
)

func TestMetricsPrometheusExposition(t *testing.T) {
	m := &Metrics{}
	m.IncClaimed()
	m.IncClaimed()
	m.IncCompleted()
	m.IncSealed()
	m.IncPurged()
	m.IncFailed()
	m.IncRecovered()
	m.IncDLQRouted()

	var sb strings.Builder
	m.WritePrometheus(&sb)
	out := sb.String()
	for _, name := range []string{
		"coldharbor_worker_claimed_total 2",
		"coldharbor_worker_completed_total 1",
		"coldharbor_worker_failed_total 1",
		"coldharbor_worker_recovered_total 1",
		"coldharbor_worker_sealed_total 1",
		"coldharbor_worker_purged_total 1",
		"coldharbor_worker_dlq_routed_total 1",
	} {
		if !strings.Contains(out, name) {
			t.Fatalf("expected exposition to contain %q, got:\n%s", name, out)
		}
	}
}
