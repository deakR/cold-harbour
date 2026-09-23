package main

import (
	"os"

	"coldharbour/internal/detect"
)

func main() {
	if err := os.WriteFile("internal/detect/testdata/realistic.jsonl", detect.GenerateRealistic(7), 0o600); err != nil {
		panic(err)
	}
}
