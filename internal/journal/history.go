package journal

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

type JobResult struct {
	ID           string
	TenantID     string
	JobType      string
	Result       map[string]any
	History      History
	Signature    []byte
	SigningKeyID string
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

func terminal(id string, result map[string]any, to JobState) JobResult {
	now := time.Now()
	return JobResult{
		ID:     id,
		Result: result,
		History: History{steps: []Transition{
			{From: CREATED, To: RUNNING, At: now},
			{From: RUNNING, To: to, At: now},
		}},
	}
}

func Succeed(id string, result map[string]any) JobResult {
	return terminal(id, result, COMPLETED)
}

func Fail(id string, result map[string]any) JobResult {
	return terminal(id, result, FAILED)
}

func FormatJobLine(result JobResult) string {
	return result.ID + " " + string(BodyOf(result.Result))
}
