package mask

import (
	"context"
	"fmt"

	"coldharbour/internal/runner"
)

type Runner struct{}

func (Runner) JobType() string { return "mask" }

func (Runner) Run(ctx context.Context, input map[string]any, cp *runner.CheckpointRecorder) (map[string]any, error) {
	_ = ctx
	if step, partial, ok := cp.Saved(); ok && step >= 1 {
		return partial, nil
	}
	doc, ok := input["document"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mask: document must be an object")
	}
	rawFields, ok := input["fields"].([]any)
	if !ok {
		return nil, fmt.Errorf("mask: fields must be an array")
	}
	masked := make(map[string]any, len(doc))
	for k, v := range doc {
		masked[k] = v
	}
	for _, raw := range rawFields {
		name, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("mask: field names must be strings")
		}
		if _, exists := masked[name]; exists {
			masked[name] = "[MASKED]"
		}
	}
	out := map[string]any{"document": masked}
	if err := cp.Save(1, out); err != nil {
		return nil, err
	}
	return out, nil
}
