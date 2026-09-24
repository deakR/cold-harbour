package redact

import (
	"context"
	"encoding/json"
	"fmt"

	"coldharbour/internal/detect"
	"coldharbour/internal/policy"
	"coldharbour/internal/runner"
)

type Loader func(ctx context.Context, tenantID string) (policy.Doc, bool, error)

type Runner struct {
	Load Loader
}

func (Runner) JobType() string { return "redact" }

func (r Runner) Run(ctx context.Context, input map[string]any, cp *runner.CheckpointRecorder) (map[string]any, error) {
	_ = ctx
	text, ok := input["input"].(string)
	if !ok {
		return nil, fmt.Errorf("redact: input[\"input\"] must be a string")
	}
	if r.Load != nil {
		if tenant, _ := input["tenantId"].(string); tenant != "" {
			doc, found, err := r.Load(ctx, tenant)
			if err != nil {
				return nil, err
			}
			if found {
				return MapResult(applyPolicy(text, doc)), nil
			}
		}
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
		partial = applyClassWindow(RedactResult{RedactedText: text}, 0)
		if err := cp.Save(1, MapResult(partial)); err != nil {
			return nil, err
		}
	}

	done := applyClassWindow(partial, 1)
	return MapResult(done), nil
}

func applyPolicy(text string, doc policy.Doc) RedactResult {
	if doc.Mode == string(detect.ModePartial) {
		masked, counts := detect.Apply(text, detect.Scan(text, doc.Detectors), detect.ModePartial)
		return RedactResult{
			RedactedText: masked,
			Counts: RedactCounts{
				EmailsRedacted: counts[detect.KindEmail],
				PhonesRedacted: counts[detect.KindPhoneUS],
				SSNRedacted:    counts[detect.KindSSN],
			},
			PolicyVersion: doc.Version,
		}
	}
	var counts RedactCounts
	masked := applySpans(text, &counts, detect.Scan(text, doc.Detectors))
	return RedactResult{RedactedText: masked, Counts: counts, PolicyVersion: doc.Version}
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
