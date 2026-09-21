package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

type Job struct {
	ID    string
	Input string
}

type JobResult struct {
	ID      string
	Result  RedactResult
	History History
}

type JobState string

const (
	CREATED   JobState = "CREATED"
	RUNNING   JobState = "RUNNING"
	COMPLETED JobState = "COMPLETED"
	FAILED    JobState = "FAILED"
)

type Transition struct {
	From JobState
	To   JobState
	At   time.Time
}

type History struct {
	steps []Transition
}

func (h History) Transitions() []Transition {
	return slices.Clone(h.steps)
}

func (h History) String() string {
	parts := make([]string, len(h.steps))
	for i, step := range h.steps {
		parts[i] = fmt.Sprintf("%s→%s (%s)", step.From, step.To, step.At.Format("15:04:05"))
	}
	return strings.Join(parts, " → ")
}

type jobMachine struct {
	steps []Transition
}

func newJobMachine() *jobMachine {
	return &jobMachine{}
}

func (m *jobMachine) pickup(at time.Time) {
	m.steps = append(m.steps, Transition{From: CREATED, To: RUNNING, At: at})
}

func (m *jobMachine) complete(at time.Time) {
	m.steps = append(slices.Clone(m.steps), Transition{From: RUNNING, To: COMPLETED, At: at})
}

func (m *jobMachine) history() History {
	return History{steps: slices.Clone(m.steps)}
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

func processJob(job Job) JobResult {
	machine := newJobMachine()
	machine.pickup(time.Now())
	redacted, err := RedactPII(job.Input)
	if err != nil {
		panic(err)
	}
	machine.complete(time.Now())
	return JobResult{ID: job.ID, Result: redacted, History: machine.history()}
}

func runWorker(jobs <-chan Job, results chan<- JobResult) {
	for job := range jobs {
		results <- processJob(job)
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
