package runner_test

import (
	"testing"

	"coldharbour/internal/mask"
	"coldharbour/internal/redact"
	"coldharbour/internal/runner"
)

func TestRegistryListsRedactAndMask(t *testing.T) {
	reg := runner.NewRegistry(redact.Runner{}, mask.Runner{})
	if _, ok := reg.Get("redact"); !ok {
		t.Fatal("redact is not registered")
	}
	if _, ok := reg.Get("mask"); !ok {
		t.Fatal("mask is not registered")
	}
	if got := reg.Types(); len(got) != 2 {
		t.Fatalf("Types() = %v, want redact and mask", got)
	}
}
