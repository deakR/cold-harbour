package queue

import (
	"reflect"
	"regexp"
	"testing"

	"coldharbour/internal/journal"
	"coldharbour/internal/redact"
)

func TestRunWorker(t *testing.T) {
	jobs := make(chan Job, len(DemoJobs))
	results := make(chan journal.JobResult)
	go runWorker(jobs, results)
	for _, job := range DemoJobs {
		jobs <- job
	}
	close(jobs)
	var got []journal.JobResult
	for result := range results {
		got = append(got, result)
	}
	if len(got) != 5 {
		t.Fatalf("got %d results, want 5", len(got))
	}
	for i := range DemoJobs {
		if got[i].ID != DemoJobs[i].ID {
			t.Errorf("got[%d].ID = %q, want %q", i, got[i].ID, DemoJobs[i].ID)
		}
		want, err := redact.RedactPII(DemoJobs[i].Input)
		if err != nil {
			t.Fatalf("RedactPII(%q) err = %v", DemoJobs[i].Input, err)
		}
		if !reflect.DeepEqual(got[i].Result, redact.MapResult(want)) {
			t.Errorf("got[%d].Result = %+v, want %+v", i, got[i].Result, redact.MapResult(want))
		}
	}
}

func TestRunWorkerHistory(t *testing.T) {
	jobs := make(chan Job, len(DemoJobs))
	results := make(chan journal.JobResult)
	go runWorker(jobs, results)
	for _, job := range DemoJobs {
		jobs <- job
	}
	close(jobs)
	var got []journal.JobResult
	for result := range results {
		got = append(got, result)
	}
	if len(got) != 5 {
		t.Fatalf("got %d results, want 5", len(got))
	}
	formatRE := regexp.MustCompile(`^CREATED→RUNNING \(\d{2}:\d{2}:\d{2}\) → RUNNING→COMPLETED \(\d{2}:\d{2}:\d{2}\)$`)
	for i, result := range got {
		edges := result.History.Transitions()
		if len(edges) != 2 {
			t.Fatalf("got[%d] history len = %d, want 2", i, len(edges))
		}
		if edges[0].From != journal.CREATED || edges[0].To != journal.RUNNING {
			t.Fatalf("got[%d] first edge = %s→%s, want CREATED→RUNNING", i, edges[0].From, edges[0].To)
		}
		if edges[1].From != journal.RUNNING || edges[1].To != journal.COMPLETED {
			t.Fatalf("got[%d] second edge = %s→%s, want RUNNING→COMPLETED", i, edges[1].From, edges[1].To)
		}
		if edges[0].At.IsZero() || edges[1].At.IsZero() {
			t.Fatalf("got[%d] history timestamps must be set", i)
		}
		formatted := result.History.String()
		if !formatRE.MatchString(formatted) {
			t.Fatalf("got[%d] history format = %q", i, formatted)
		}
	}
}
