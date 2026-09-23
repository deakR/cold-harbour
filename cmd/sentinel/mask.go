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
	return maskWith(line, maxInline, shadow, maskKinds, detect.ModeRedact, nil, "closed")
}

func maskWith(line string, maxInline int, shadow bool, kinds []detect.Kind, mode detect.Mode, h *handoff, failPolicy string) maskResult {
	if len(kinds) == 0 {
		kinds = maskKinds
	}
	if mode == "" {
		mode = detect.ModeRedact
	}
	if failPolicy == "" {
		failPolicy = "closed"
	}
	if len(line) > maxInline {
		if shadow {
			counts := map[detect.Kind]int{}
			for _, sp := range detect.Scan(line, kinds) {
				counts[sp.Kind]++
			}
			return maskResult{line: line, counts: counts}
		}
		return h.handle(line, failPolicy, kinds, mode)
	}
	spans := detect.Scan(line, kinds)
	if shadow {
		counts := map[detect.Kind]int{}
		for _, sp := range spans {
			counts[sp.Kind]++
		}
		return maskResult{line: line, counts: counts}
	}
	out, counts := detect.Apply(line, spans, mode)
	return maskResult{line: out, counts: counts}
}
