package runner

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
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

//go:embed types.txt
var typeList string

// ListedTypes is the job-type allowlist shared with the control plane.
func ListedTypes() []string {
	var out []string
	for _, line := range strings.Split(typeList, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// RequireListedTypes fails when the registry and types.txt disagree.
func (r *Registry) RequireListedTypes() error {
	listed := ListedTypes()
	n := 0
	if r != nil {
		n = len(r.byType)
	}
	if n != len(listed) {
		return fmt.Errorf("registered %d job types, types.txt lists %d", n, len(listed))
	}
	for _, name := range listed {
		if _, ok := r.Get(name); !ok {
			return fmt.Errorf("types.txt lists %s, which is not registered", name)
		}
	}
	return nil
}
