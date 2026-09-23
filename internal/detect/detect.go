package detect

import (
	"regexp"
	"sort"
	"strings"
)

type Kind string

const (
	KindEmail   Kind = "email"
	KindPhoneUS Kind = "phone_us"
	KindPhoneIN Kind = "phone_in"
	KindSSN     Kind = "ssn"
	KindAadhaar Kind = "aadhaar"
	KindPAN     Kind = "pan"
)

type Span struct {
	Kind       Kind
	Start, End int
}

type Mode string

const (
	ModeRedact  Mode = "redact"
	ModePartial Mode = "partial"
)

type detector struct {
	kind Kind
	find func(string) []Span
}

var detectors = []detector{
	{KindEmail, regexSpans(KindEmail, regexp.MustCompile(`[\w.]+@[\w.]+\.[A-Za-z]+`))},
	{KindPhoneUS, regexSpans(KindPhoneUS, regexp.MustCompile(`\(\d{3}\) \d{3}-\d{4}|\d{3}-\d{3}-\d{4}|\+1 \d{3} \d{3} \d{4}`))},
	{KindSSN, regexSpans(KindSSN, regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`))},
	{KindPhoneIN, findPhoneIN},
	{KindAadhaar, findAadhaar},
	{KindPAN, findPAN},
}

func regexSpans(kind Kind, re *regexp.Regexp) func(string) []Span {
	return func(s string) []Span {
		locs := re.FindAllStringIndex(s, -1)
		out := make([]Span, 0, len(locs))
		for _, loc := range locs {
			out = append(out, Span{Kind: kind, Start: loc[0], End: loc[1]})
		}
		return out
	}
}

func Scan(s string, enabled []Kind) []Span {
	allow := make(map[Kind]struct{}, len(enabled))
	for _, kind := range enabled {
		allow[kind] = struct{}{}
	}
	var spans []Span
	for _, d := range detectors {
		if _, ok := allow[d.kind]; !ok {
			continue
		}
		spans = append(spans, d.find(s)...)
	}
	return preferLonger(spans)
}

func preferLonger(spans []Span) []Span {
	sort.Slice(spans, func(i, j int) bool {
		li := spans[i].End - spans[i].Start
		lj := spans[j].End - spans[j].Start
		if li != lj {
			return li > lj
		}
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].Kind < spans[j].Kind
	})
	chosen := make([]Span, 0, len(spans))
	for _, sp := range spans {
		if sp.End <= sp.Start {
			continue
		}
		hit := false
		for _, c := range chosen {
			if sp.Start < c.End && c.Start < sp.End {
				hit = true
				break
			}
		}
		if !hit {
			chosen = append(chosen, sp)
		}
	}
	sort.Slice(chosen, func(i, j int) bool { return chosen[i].Start < chosen[j].Start })
	return chosen
}

func Apply(s string, spans []Span, mode Mode) (string, map[Kind]int) {
	ordered := append([]Span(nil), spans...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start < ordered[j].Start })
	counts := map[Kind]int{}
	var b strings.Builder
	prev := 0
	for _, sp := range ordered {
		if sp.Start < prev || sp.End > len(s) || sp.End <= sp.Start {
			continue
		}
		b.WriteString(s[prev:sp.Start])
		if mode == ModePartial {
			b.WriteString(partial(s[sp.Start:sp.End]))
		} else {
			b.WriteString("[" + strings.ToUpper(string(sp.Kind)) + "]")
		}
		counts[sp.Kind]++
		prev = sp.End
	}
	b.WriteString(s[prev:])
	return b.String(), counts
}

func partial(s string) string {
	if len(s) <= 4 {
		return s
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}
