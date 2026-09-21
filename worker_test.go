package main

import "testing"

func TestRunWorker(t *testing.T) {
	jobs := make(chan Job, len(demoJobs))
	results := make(chan JobResult)
	go runWorker(jobs, results)
	for _, job := range demoJobs {
		jobs <- job
	}
	close(jobs)
	var got []JobResult
	for result := range results {
		got = append(got, result)
	}
	if len(got) != 5 {
		t.Fatalf("got %d results, want 5", len(got))
	}
	for i := range demoJobs {
		if got[i].ID != demoJobs[i].ID {
			t.Errorf("got[%d].ID = %q, want %q", i, got[i].ID, demoJobs[i].ID)
		}
		want, err := RedactPII(demoJobs[i].Input)
		if err != nil {
			t.Fatalf("RedactPII(%q) err = %v", demoJobs[i].Input, err)
		}
		if got[i].Result != want {
			t.Errorf("got[%d].Result = %+v, want %+v", i, got[i].Result, want)
		}
	}
}
