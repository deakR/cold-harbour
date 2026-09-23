package redact

import (
	"context"
	"strings"
	"testing"

	"coldharbour/internal/detect"
	"coldharbour/internal/policy"
)

func TestPolicyChangesNextJob(t *testing.T) {
	r := Runner{Load: func(context.Context, string) (policy.Doc, bool, error) {
		return policy.Doc{
			Version:        2,
			Detectors:      []detect.Kind{detect.KindAadhaar},
			Mode:           "redact",
			FailPolicy:     "closed",
			MaxInlineBytes: 65536,
		}, true, nil
	}}
	out, err := r.Run(context.Background(), map[string]any{
		"input":    "id 234123412346 ok",
		"tenantId": "tenant-1",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	text, _ := out["redactedText"].(string)
	if strings.Contains(text, "234123412346") || !strings.Contains(text, "[AADHAAR]") {
		t.Fatal("policy did not mask aadhaar")
	}
	if out["policyVersion"] != float64(2) {
		t.Fatalf("policyVersion = %v", out["policyVersion"])
	}
}
