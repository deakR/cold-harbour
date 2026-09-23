package main

import "coldharbour/internal/detect"

var maskKinds = []detect.Kind{
	detect.KindEmail,
	detect.KindPhoneUS,
	detect.KindPhoneIN,
	detect.KindSSN,
	detect.KindAadhaar,
	detect.KindPAN,
}

type maskResult struct {
	line    string
	counts  map[detect.Kind]int
	dropped bool
}

func maskLine(line string, maxInline int, shadow bool) maskResult {
	if len(line) > maxInline {
		return maskResult{line: "[DROPPED reason=too_large]", dropped: true}
	}
	spans := detect.Scan(line, maskKinds)
	if shadow {
		counts := map[detect.Kind]int{}
		for _, sp := range spans {
			counts[sp.Kind]++
		}
		return maskResult{line: line, counts: counts}
	}
	out, counts := detect.Apply(line, spans, detect.ModeRedact)
	return maskResult{line: out, counts: counts}
}
