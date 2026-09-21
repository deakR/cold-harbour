package main

import "encoding/json"

type Job struct {
	ID    string
	Input string
}

type JobResult struct {
	ID     string
	Result RedactResult
}

const m1Fixture = `Contact jane.doe@example.com or (555) 123-4567 for details.
SSN on file: 123-45-6789. Backup contact: john@company.org.
Not a match: version 123-45 or year 1234-56-789.`

var demoJobs = []Job{
	{ID: "job-5", Input: m1Fixture},
	{ID: "job-1", Input: "Email alice.nguyen@school.edu about the lab report."},
	{ID: "job-4", Input: "Call 415-555-0199 before noon."},
	{ID: "job-2", Input: "Employee SSN 987-65-4321 is on the form."},
	{ID: "job-3", Input: "Reach +1 212 555 0100 or bob.smith@mail.net."},
}

func runWorker(jobs <-chan Job, results chan<- JobResult) {
	for range jobs {
	}
	close(results)
}

func formatJobLine(result JobResult) string {
	b, err := json.Marshal(result.Result)
	if err != nil {
		panic(err)
	}
	return result.ID + " " + string(b)
}
