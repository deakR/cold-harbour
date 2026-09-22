package redact

import (
	"context"
	"encoding/json"
	"fmt"

	"coldharbour/internal/runner"
)

type Runner struct{}

func (Runner) JobType() string { return "redact" }

func (Runner) Run(ctx context.Context, input map[string]any, cp *runner.CheckpointRecorder) (map[string]any, error) {
	_ = ctx
	text, ok := input["input"].(string)
	if !ok {
		return nil, fmt.Errorf("redact: input[\"input\"] must be a string")
	}

	var partial RedactResult
	step, saved, hasSaved := cp.Saved()
	if hasSaved && step >= 1 {
		var err error
		partial, err = resultFromMap(saved)
		if err != nil {
			return nil, err
		}
	} else {
		partial = ApplyClassWindow(RedactResult{RedactedText: text}, 0)
		if err := cp.Save(1, MapResult(partial)); err != nil {
			return nil, err
		}
	}

	done := ApplyClassWindow(partial, 1)
	return MapResult(done), nil
}

func MapResult(result RedactResult) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(MarshalResult(result), &m); err != nil {
		panic(err)
	}
	return m
}

func resultFromMap(m map[string]any) (RedactResult, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return RedactResult{}, err
	}
	var r RedactResult
	if err := json.Unmarshal(b, &r); err != nil {
		return RedactResult{}, err
	}
	return r, nil
}
