package redact

import (
	"strings"
	"testing"
)

func TestRedactPII(t *testing.T) {
	got, err := RedactPII(M1Fixture)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got.Counts.EmailsRedacted != 2 {
		t.Errorf("EmailsRedacted = %d, want 2", got.Counts.EmailsRedacted)
	}
	if got.Counts.PhonesRedacted != 1 {
		t.Errorf("PhonesRedacted = %d, want 1", got.Counts.PhonesRedacted)
	}
	if got.Counts.SSNRedacted != 1 {
		t.Errorf("SSNRedacted = %d, want 1", got.Counts.SSNRedacted)
	}
	if !strings.Contains(got.RedactedText, "version 123-45") {
		t.Errorf("RedactedText missing %q: %q", "version 123-45", got.RedactedText)
	}
	if !strings.Contains(got.RedactedText, "year 1234-56-789") {
		t.Errorf("RedactedText missing %q: %q", "year 1234-56-789", got.RedactedText)
	}
	if !strings.Contains(got.RedactedText, "[EMAIL_REDACTED]") {
		t.Errorf("RedactedText missing %q: %q", "[EMAIL_REDACTED]", got.RedactedText)
	}
	if !strings.Contains(got.RedactedText, "[PHONE_REDACTED]") {
		t.Errorf("RedactedText missing %q: %q", "[PHONE_REDACTED]", got.RedactedText)
	}
	if !strings.Contains(got.RedactedText, "[SSN_REDACTED]") {
		t.Errorf("RedactedText missing %q: %q", "[SSN_REDACTED]", got.RedactedText)
	}
}
