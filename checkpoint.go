package main

import "errors"

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
	PartialResult RedactResult
}

type CheckpointStore struct {
	byJob       map[string]Checkpoint
	step1Passes map[string]int
}

type classWindow struct {
	start int
	end   int
}

var stepWindows = [...]classWindow{
	{0, 2},
	{2, 3},
}

func init() {
	if stepWindows[0].start != 0 {
		panic("step windows must start at classes index 0")
	}
	end := 0
	for _, w := range stepWindows {
		if w.start != end || w.end <= w.start {
			panic("step windows must be contiguous half-open ranges")
		}
		end = w.end
	}
	if end != len(classes) {
		panic("step windows must cover every classes entry")
	}
}

func NewCheckpointStore() *CheckpointStore {
	return &CheckpointStore{
		byJob:       make(map[string]Checkpoint),
		step1Passes: make(map[string]int),
	}
}

func (s *CheckpointStore) Run(job Job, simulateCrashAtStep CrashPoint) (RedactResult, error) {
	if job.ID == "" {
		return RedactResult{}, ErrEmptyJobID
	}
	if _, exists := s.byJob[job.ID]; exists {
		return RedactResult{}, ErrJobExists
	}

	partial := applyClassWindow(RedactResult{RedactedText: job.Input}, stepWindows[0])
	s.step1Passes[job.ID]++
	s.save(Checkpoint{JobID: job.ID, Step: StepEmailsAndPhones, PartialResult: partial})
	if simulateCrashAtStep == CrashAfterStep1 {
		return RedactResult{}, ErrSimulatedCrash
	}

	done := applyClassWindow(partial, stepWindows[1])
	s.save(Checkpoint{JobID: job.ID, Step: StepSSNs, PartialResult: done})
	return done, nil
}

func (s *CheckpointStore) Resume(jobID string) (RedactResult, error) {
	if jobID == "" {
		return RedactResult{}, ErrEmptyJobID
	}
	cp, ok := s.byJob[jobID]
	if !ok {
		return RedactResult{}, ErrUnknownJob
	}
	switch cp.Step {
	case StepSSNs:
		return cp.PartialResult, nil
	case StepEmailsAndPhones:
		done := applyClassWindow(cp.PartialResult, stepWindows[1])
		s.save(Checkpoint{JobID: jobID, Step: StepSSNs, PartialResult: done})
		return done, nil
	default:
		return RedactResult{}, ErrUnknownJob
	}
}

func (s *CheckpointStore) Step1Passes(jobID string) int {
	return s.step1Passes[jobID]
}

func (s *CheckpointStore) save(cp Checkpoint) {
	s.byJob[cp.JobID] = cp
}

func applyClassWindow(in RedactResult, w classWindow) RedactResult {
	text := in.RedactedText
	counts := in.Counts
	for i := w.start; i < w.end; i++ {
		text = applyClass(text, &counts, &classes[i])
	}
	return RedactResult{RedactedText: text, Counts: counts}
}
