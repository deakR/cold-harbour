package main

import (
	"strings"
	"testing"
)

func TestMaskSyntheticEmailLine(t *testing.T) {
	line := makeLine(0)
	const raw = "user0@example.com"
	if !strings.Contains(line, raw) {
		t.Fatalf("makeLine(0) missing %q", raw)
	}
	out := maskLine(line)
	if strings.Contains(out, raw) {
		t.Fatalf("raw email remained in masked output")
	}
	if !strings.Contains(out, "[EMAIL]") {
		t.Fatalf("masked output missing [EMAIL]")
	}
}
