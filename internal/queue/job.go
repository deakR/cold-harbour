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

func processJob(job Job) journal.JobResult {
	redacted, err := redact.RedactPII(job.Input)
	if err != nil {
		panic(err)
	}
	result := journal.Succeed(job.ID, redact.MapResult(redacted))
	result.JobType = "redact"
	return result
}

func runWorker(jobs <-chan Job, results chan<- journal.JobResult) {
	for job := range jobs {
		results <- processJob(job)
	}
	close(results)
}
