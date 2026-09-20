package main

type RedactResult struct {
	RedactedText string       `json:"redactedText"`
	Counts       RedactCounts `json:"counts"`
}

type RedactCounts struct {
	EmailsRedacted int `json:"emailsRedacted"`
	PhonesRedacted int `json:"phonesRedacted"`
	SSNRedacted    int `json:"ssnRedacted"`
}

func RedactPII(input string) (RedactResult, error) {
	return RedactResult{}, nil
}
