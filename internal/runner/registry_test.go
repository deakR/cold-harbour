package runner_test

import (
	"testing"

	"coldharbour/internal/mask"
	"coldharbour/internal/redact"
	"coldharbour/internal/runner"
)

func TestRegistryMatchesTypeFile(t *testing.T) {
	reg := runner.NewRegistry(redact.Runner{}, mask.Runner{})
	if err := reg.RequireListedTypes(); err != nil {
		t.Fatal(err)
	}
}
