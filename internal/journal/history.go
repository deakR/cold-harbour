package journal

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"coldharbour/internal/redact"
)

type JobResult struct {
	ID           string
	TenantID     string
	Result       redact.RedactResult
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

func (m *jobMachine) fail(at time.Time) {
	m.steps = append(slices.Clone(m.steps), Transition{From: RUNNING, To: FAILED, At: at})
}

func (m *jobMachine) history() History {
	return History{steps: slices.Clone(m.steps)}
}

func Succeed(id string, result redact.RedactResult) JobResult {
	machine := newJobMachine()
	machine.pickup(time.Now())
	machine.complete(time.Now())
	return JobResult{ID: id, Result: result, History: machine.history()}
}

func Fail(id string, result redact.RedactResult) JobResult {
	machine := newJobMachine()
	machine.pickup(time.Now())
	machine.fail(time.Now())
	return JobResult{ID: id, Result: result, History: machine.history()}
}

func FormatJobLine(result JobResult) string {
	return result.ID + " " + string(redact.MarshalResult(result.Result))
}
