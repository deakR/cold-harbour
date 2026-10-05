package detect

import (
	"testing"
)

func TestCreditCard(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"4532 0150 1234 5671", true},       // Valid Visa
		{"5425 2334 3010 9903", true},       // Valid Mastercard
		{"3782 822463 10005", true},         // Valid Amex
		{"4532-0150-1234-5671", true},       // Valid Visa with dashes
		{"4532015012345671", true},          // Valid Visa continuous
		{"4532 0150 1234 5670", false},      // Invalid checksum
		{"1234 5678 9012 3456", false},      // Invalid IIN (starts with 1)
		{"12345", false},                    // Too short
		{"version 453201501234567499999", false}, // Too long
	}

	for _, tc := range cases {
		spans := findCreditCard(tc.input)
		got := len(spans) > 0
		if got != tc.want {
			t.Errorf("findCreditCard(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIP(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"Server IP 192.168.1.1 connected", true},
		{"Public 8.8.8.8 dns", true},
		{"IPv6 2001:0db8:85a3:0000:0000:8a2e:0370:7334 active", true},
		{"Local loopback ::1", true},
		{"Not an IP: 999.999.999.999", false},
		{"Not an IP: 1.2.3", false},
	}

	for _, tc := range cases {
		spans := findIP(tc.input)
		got := len(spans) > 0
		if got != tc.want {
			t.Errorf("findIP(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestModeTokenize(t *testing.T) {
	text := "Contact user at test@example.com."
	spans := Scan(text, []Kind{KindEmail})
	redacted, counts := Apply(text, spans, ModeTokenize)

	if counts[KindEmail] != 1 {
		t.Fatalf("expected 1 email counted, got %d", counts[KindEmail])
	}
	if len(redacted) == len(text) {
		t.Errorf("expected tokenized text to differ from input")
	}
	// Verify deterministic consistency
	redacted2, _ := Apply(text, spans, ModeTokenize)
	if redacted != redacted2 {
		t.Errorf("tokenization must be deterministic: %q != %q", redacted, redacted2)
	}
}
