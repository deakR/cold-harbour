package redact

import (
	"encoding/json"
	"regexp"
)

const M1Fixture = `Contact jane.doe@example.com or (555) 123-4567 for details.
SSN on file: 123-45-6789. Backup contact: john@company.org.
Not a match: version 123-45 or year 1234-56-789.`

type RedactResult struct {
	RedactedText string       `json:"redactedText"`
	Counts       RedactCounts `json:"counts"`
}

type RedactCounts struct {
	EmailsRedacted int `json:"emailsRedacted"`
	PhonesRedacted int `json:"phonesRedacted"`
	SSNRedacted    int `json:"ssnRedacted"`
}

type patternClass struct {
	re          *regexp.Regexp
	placeholder string
	countField  func(*RedactCounts) *int
}

var classes = []patternClass{
	{
		re:          regexp.MustCompile(`[\w.]+@[\w.]+\.[A-Za-z]+`),
		placeholder: "[EMAIL_REDACTED]",
		countField:  func(c *RedactCounts) *int { return &c.EmailsRedacted },
	},
	{
		re:          regexp.MustCompile(`\(\d{3}\) \d{3}-\d{4}|\d{3}-\d{3}-\d{4}|\+1 \d{3} \d{3} \d{4}`),
		placeholder: "[PHONE_REDACTED]",
		countField:  func(c *RedactCounts) *int { return &c.PhonesRedacted },
	},
	{
		re:          regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		placeholder: "[SSN_REDACTED]",
		countField:  func(c *RedactCounts) *int { return &c.SSNRedacted },
	},
}

type classWindow struct {
	start int
	end   int
}

var stepWindows = [...]classWindow{
	{0, 2},
	{2, 3},
}

func init() {
	if stepWindows[0].start != 0 {
		panic("step windows must start at classes index 0")
	}
	end := 0
	for _, w := range stepWindows {
		if w.start != end || w.end <= w.start {
			panic("step windows must be contiguous half-open ranges")
		}
		end = w.end
	}
	if end != len(classes) {
		panic("step windows must cover every classes entry")
	}
}

func applyClass(text string, counts *RedactCounts, pc *patternClass) string {
	n := 0
	text = pc.re.ReplaceAllStringFunc(text, func(string) string {
		n++
		return pc.placeholder
	})
	*pc.countField(counts) = n
	return text
}

func applyClassWindow(in RedactResult, w classWindow) RedactResult {
	text := in.RedactedText
	counts := in.Counts
	for i := w.start; i < w.end; i++ {
		text = applyClass(text, &counts, &classes[i])
	}
	return RedactResult{RedactedText: text, Counts: counts}
}

func ApplyClassWindow(in RedactResult, windowIndex int) RedactResult {
	return applyClassWindow(in, stepWindows[windowIndex])
}

func RedactPII(input string) (RedactResult, error) {
	text := input
	var counts RedactCounts
	for i := range classes {
		text = applyClass(text, &counts, &classes[i])
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
