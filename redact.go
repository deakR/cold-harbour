package main

import "regexp"

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

func RedactPII(input string) (RedactResult, error) {
	text := input
	var counts RedactCounts
	for i := range classes {
		pc := &classes[i]
		n := 0
		text = pc.re.ReplaceAllStringFunc(text, func(string) string {
			n++
			return pc.placeholder
		})
		*pc.countField(&counts) = n
	}
	return RedactResult{RedactedText: text, Counts: counts}, nil
}
