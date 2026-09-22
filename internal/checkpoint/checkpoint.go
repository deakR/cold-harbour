package checkpoint

import (
	"errors"

	"coldharbour/internal/redact"
)

var (
	ErrSimulatedCrash = errors.New("simulated crash after step 1 checkpoint")
	ErrUnknownJob     = errors.New("no checkpoint for job")
	ErrJobExists      = errors.New("checkpoint already exists for job")
	ErrEmptyJobID     = errors.New("empty job id")
)

type Step int

const (
	StepEmailsAndPhones Step = 1
	StepSSNs            Step = 2
)

type CrashPoint int

const (
	CrashNever      CrashPoint = 0
	CrashAfterStep1 CrashPoint = 1
)

type Checkpoint struct {
	JobID         string
	Step          Step
	PartialResult redact.RedactResult
}

type CheckpointStore struct {
	byJob       map[string]Checkpoint
	step1Passes map[string]int
}

func NewCheckpointStore() *CheckpointStore {
	return &CheckpointStore{
		byJob:       make(map[string]Checkpoint),
		step1Passes: make(map[string]int),
	}
}

func (s *CheckpointStore) Run(jobID, input string, simulateCrashAtStep CrashPoint) (redact.RedactResult, error) {
	if jobID == "" {
		return redact.RedactResult{}, ErrEmptyJobID
	}
	if _, exists := s.byJob[jobID]; exists {
		return redact.RedactResult{}, ErrJobExists
	}

	partial := redact.ApplyClassWindow(redact.RedactResult{RedactedText: input}, 0)
	s.step1Passes[jobID]++
	s.save(Checkpoint{JobID: jobID, Step: StepEmailsAndPhones, PartialResult: partial})
	if simulateCrashAtStep == CrashAfterStep1 {
		return redact.RedactResult{}, ErrSimulatedCrash
	}

	done := redact.ApplyClassWindow(partial, 1)
	s.save(Checkpoint{JobID: jobID, Step: StepSSNs, PartialResult: done})
	return done, nil
}

func (s *CheckpointStore) Resume(jobID string) (redact.RedactResult, error) {
	if jobID == "" {
		return redact.RedactResult{}, ErrEmptyJobID
	}
	cp, ok := s.byJob[jobID]
	if !ok {
		return redact.RedactResult{}, ErrUnknownJob
	}
	switch cp.Step {
	case StepSSNs:
		return cp.PartialResult, nil
	case StepEmailsAndPhones:
		done := redact.ApplyClassWindow(cp.PartialResult, 1)
		s.save(Checkpoint{JobID: jobID, Step: StepSSNs, PartialResult: done})
		return done, nil
	default:
		return redact.RedactResult{}, ErrUnknownJob
	}
}

func (s *CheckpointStore) Step1Passes(jobID string) int {
	return s.step1Passes[jobID]
}

func (s *CheckpointStore) save(cp Checkpoint) {
	s.byJob[cp.JobID] = cp
}
