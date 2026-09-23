package main

import (
	"os"

	"coldharbour/internal/detect"
)

func main() {
	if err := os.WriteFile("internal/detect/testdata/corpus.jsonl", detect.GenerateCorpus(1), 0o600); err != nil {
		panic(err)
	}
}
