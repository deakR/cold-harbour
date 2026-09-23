package runner

import (
	"context"
	"slices"
)

type CheckpointRecorder struct {
	saved func() (step int, partial map[string]any, ok bool)
	save  func(step int, partial map[string]any) error
}

func NewCheckpointRecorder(
	saved func() (step int, partial map[string]any, ok bool),
	save func(step int, partial map[string]any) error,
) *CheckpointRecorder {
	return &CheckpointRecorder{saved: saved, save: save}
}

func (c *CheckpointRecorder) Saved() (step int, partial map[string]any, ok bool) {
	if c == nil || c.saved == nil {
		return 0, nil, false
	}
	return c.saved()
}

func (c *CheckpointRecorder) Save(step int, partial map[string]any) error {
	if c == nil || c.save == nil {
		return nil
	}
	return c.save(step, partial)
}

type JobRunner interface {
	JobType() string
	Run(ctx context.Context, input map[string]any, cp *CheckpointRecorder) (map[string]any, error)
}

type Registry struct {
	byType map[string]JobRunner
}

func NewRegistry(runners ...JobRunner) *Registry {
	r := &Registry{byType: make(map[string]JobRunner, len(runners))}
	for _, jr := range runners {
		r.byType[jr.JobType()] = jr
	}
	return r
}

func (r *Registry) Get(jobType string) (JobRunner, bool) {
	if r == nil {
		return nil, false
	}
	jr, ok := r.byType[jobType]
	return jr, ok
}

func (r *Registry) Types() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.byType))
	for name := range r.byType {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}
