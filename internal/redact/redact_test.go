package redact

import (
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
	if got.RedactedText != `Contact [EMAIL_REDACTED] or [PHONE_REDACTED] for details.
SSN on file: [SSN_REDACTED]. Backup contact: [EMAIL_REDACTED].
Not a match: version 123-45 or year 1234-56-789.` {
		t.Errorf("RedactedText = %q", got.RedactedText)
	}
}
