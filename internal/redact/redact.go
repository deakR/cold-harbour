package redact

import (
	"encoding/json"
	"strings"

	"coldharbour/internal/detect"
)

const M1Fixture = `Contact jane.doe@example.com or (555) 123-4567 for details.
SSN on file: 123-45-6789. Backup contact: john@company.org.
Not a match: version 123-45 or year 1234-56-789.`

type RedactResult struct {
	RedactedText  string       `json:"redactedText"`
	Counts        RedactCounts `json:"counts"`
	PolicyVersion int          `json:"policyVersion"`
}

type RedactCounts struct {
	EmailsRedacted int `json:"emailsRedacted"`
	PhonesRedacted int `json:"phonesRedacted"`
	SSNRedacted    int `json:"ssnRedacted"`
}

var stepKinds = [][]detect.Kind{
	{detect.KindEmail, detect.KindPhoneUS},
	{detect.KindSSN},
}

func init() {
	seen := map[detect.Kind]int{}
	for _, kinds := range stepKinds {
		for _, kind := range kinds {
			seen[kind]++
		}
	}
	for _, kind := range []detect.Kind{detect.KindEmail, detect.KindPhoneUS, detect.KindSSN} {
		if seen[kind] != 1 {
			panic("redact windows must cover " + string(kind) + " once")
		}
	}
}

func applySpans(text string, counts *RedactCounts, spans []detect.Span) string {
	var b []byte
	prev := 0
	for _, sp := range spans {
		b = append(b, text[prev:sp.Start]...)
		b = append(b, token(sp.Kind)...)
		switch sp.Kind {
		case detect.KindEmail:
			counts.EmailsRedacted++
		case detect.KindPhoneUS:
			counts.PhonesRedacted++
		case detect.KindSSN:
			counts.SSNRedacted++
		}
		prev = sp.End
	}
	b = append(b, text[prev:]...)
	return string(b)
}

func token(kind detect.Kind) string {
	switch kind {
	case detect.KindEmail:
		return "[EMAIL_REDACTED]"
	case detect.KindPhoneUS:
		return "[PHONE_REDACTED]"
	case detect.KindSSN:
		return "[SSN_REDACTED]"
	default:
		return "[" + strings.ToUpper(string(kind)) + "]"
	}
}

func applyClassWindow(in RedactResult, windowIndex int) RedactResult {
	text := in.RedactedText
	counts := in.Counts
	spans := detect.Scan(text, stepKinds[windowIndex])
	return RedactResult{RedactedText: applySpans(text, &counts, spans), Counts: counts}
}

func RedactPII(input string) (RedactResult, error) {
	text := input
	var counts RedactCounts
	for i := range stepKinds {
		spans := detect.Scan(text, stepKinds[i])
		text = applySpans(text, &counts, spans)
	}
	return RedactResult{RedactedText: text, Counts: counts}, nil
}

func MarshalResult(result RedactResult) []byte {
	b, err := json.Marshal(result)
	if err != nil {
		panic(err)
	}
	return b
}
