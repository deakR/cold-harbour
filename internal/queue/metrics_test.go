package queue

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricCountersMove(t *testing.T) {
	completed := testutil.ToFloat64(jobsCompleted)
	failed := testutil.ToFloat64(jobsFailed)
	retried := testutil.ToFloat64(jobsRetried)
	buried := testutil.ToFloat64(jobsBuried)
	jobsCompleted.Inc()
	jobsFailed.Inc()
	jobsRetried.Inc()
	jobsBuried.Inc()
	if testutil.ToFloat64(jobsCompleted) != completed+1 {
		t.Fatal("completed counter did not move")
	}
	if testutil.ToFloat64(jobsFailed) != failed+1 {
		t.Fatal("failed counter did not move")
	}
	if testutil.ToFloat64(jobsRetried) != retried+1 {
		t.Fatal("retried counter did not move")
	}
	if testutil.ToFloat64(jobsBuried) != buried+1 {
		t.Fatal("buried counter did not move")
	}
}
