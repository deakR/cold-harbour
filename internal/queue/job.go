package queue

import (
	"coldharbour/internal/journal"
	"coldharbour/internal/redact"
)

type Job struct {
	ID       string
	Input    string
	TenantID string
}

var DemoJobs = []Job{
	{ID: "job-5", Input: redact.M1Fixture},
	{ID: "job-1", Input: "Email alice.nguyen@school.edu about the lab report."},
	{ID: "job-4", Input: "Call 415-555-0199 before noon."},
	{ID: "job-2", Input: "Employee SSN 987-65-4321 is on the form."},
	{ID: "job-3", Input: "Reach +1 212 555 0100 or bob.smith@mail.net."},
}

func processJob(job Job) journal.JobResult {
	redacted, err := redact.RedactPII(job.Input)
	if err != nil {
		panic(err)
	}
	return journal.Succeed(job.ID, redacted)
}

func runWorker(jobs <-chan Job, results chan<- journal.JobResult) {
	for job := range jobs {
		results <- processJob(job)
	}
	close(results)
}
