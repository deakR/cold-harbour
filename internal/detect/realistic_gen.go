package detect

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"strconv"
)

const (
	realisticPosPerKind = 120
	realisticNearMiss   = 320
)

// GenerateRealistic builds a Scan-aligned realistic.jsonl body.
func GenerateRealistic(seed uint64) []byte {
	rng := rand.New(rand.NewPCG(seed, seed))
	kinds := []Kind{KindEmail, KindPhoneUS, KindPhoneIN, KindSSN, KindAadhaar, KindPAN}
	var lines []corpusLine
	for _, kind := range kinds {
		kept := 0
		for attempt := 0; kept < realisticPosPerKind && attempt < realisticPosPerKind*40; attempt++ {
			value := realisticValue(rng, kind, attempt)
			text := realisticShape(rng, value, attempt)
			got := Scan(text, kinds)
			if !hasKind(got, kind) {
				continue
			}
			lines = append(lines, selfLabelFromScan(text, got))
			kept++
		}
	}
	keptEmpty := 0
	for attempt := 0; keptEmpty < realisticNearMiss && attempt < realisticNearMiss*40; attempt++ {
		text := realisticNearMissLine(rng, attempt)
		got := Scan(text, kinds)
		if len(got) != 0 {
			continue
		}
		lines = append(lines, corpusLine{Text: text, Spans: []corpusSpan{}})
		keptEmpty++
	}
	if keptEmpty < realisticNearMiss {
		panic("realistic near-miss shortfall")
	}
	for _, kind := range kinds {
		n := 0
		for _, line := range lines {
			for _, sp := range line.Spans {
				if sp.Kind == kind {
					n++
					break
				}
			}
		}
		if n < realisticPosPerKind {
			panic("realistic positive shortfall for " + string(kind))
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, line := range lines {
		if line.Spans == nil {
			line.Spans = []corpusSpan{}
		}
		_ = enc.Encode(line)
	}
	return buf.Bytes()
}

func selfLabelFromScan(text string, got []Span) corpusLine {
	spans := make([]corpusSpan, 0, len(got))
	for _, sp := range got {
		spans = append(spans, corpusSpan{Kind: sp.Kind, Start: sp.Start, End: sp.End})
	}
	return corpusLine{Text: text, Spans: spans}
}

func hasKind(spans []Span, kind Kind) bool {
	for _, sp := range spans {
		if sp.Kind == kind {
			return true
		}
	}
	return false
}

func realisticValue(rng *rand.Rand, kind Kind, n int) string {
	switch kind {
	case KindEmail:
		if n%5 == 4 {
			return "user" + strconv.Itoa(n) + "%40example.test"
		}
		return "user" + strconv.Itoa(n) + "@example.test"
	case KindPhoneUS:
		return phoneUS(rng, n%4)
	case KindPhoneIN:
		return phoneIN(rng, true, n%4)
	case KindSSN:
		return ssnNum(rng)
	case KindAadhaar:
		return formatAadhaar(aadhaarDigits(rng, true), n%3)
	case KindPAN:
		return panDigits(rng, true)
	default:
		return "x"
	}
}

func realisticShape(rng *rand.Rand, value string, n int) string {
	switch n % 6 {
	case 0:
		return `{"ts":` + strconv.Itoa(1700000000+rng.IntN(90000)) + `,"level":"info","field":"` + value + `","ok":true}`
	case 1:
		return `GET /v1/lookup?q=` + value + `&page=1 HTTP/1.1`
	case 2:
		return `10.0.0.` + strconv.Itoa(1+rng.IntN(200)) + ` - - [23/Sep/2026:08:00:00 +0000] "GET /x HTTP/1.1" 200 512 "-" "` + value + `"`
	case 3:
		return `java.lang.IllegalStateException: bad input at com.ex.App.run(App.java:` + strconv.Itoa(10+rng.IntN(80)) + `) detail=` + value
	case 4:
		return `req_id=` + strconv.Itoa(rng.IntN(1000000)) + ` status=ok value=` + value + ` region=us`
	default:
		if rng.IntN(2) == 0 {
			return `id,` + strconv.Itoa(rng.IntN(100000)) + `,` + value + `,active`
		}
		return `id|` + strconv.Itoa(rng.IntN(100000)) + `|` + value + `|active`
	}
}

func realisticNearMissLine(rng *rand.Rand, n int) string {
	switch n % 8 {
	case 0:
		return realisticShape(rng, "ORD-"+strconv.Itoa(1000000000+rng.IntN(899999999)), n)
	case 1:
		return realisticShape(rng, strconv.Itoa(1600000000+rng.IntN(200000000)), n)
	case 2:
		return realisticShape(rng, formatAadhaar(aadhaarDigits(rng, false), n%3), n)
	case 3:
		return realisticShape(rng, panDigits(rng, false), n)
	case 4:
		return realisticShape(rng, phoneIN(rng, false, 0), n)
	case 5:
		return realisticShape(rng, "build-"+strconv.Itoa(10000+rng.IntN(90000)), n)
	case 6:
		return realisticShape(rng, "v"+strconv.Itoa(1+rng.IntN(9))+"."+strconv.Itoa(rng.IntN(20))+"."+strconv.Itoa(rng.IntN(100)), n)
	default:
		return realisticShape(rng, "SKU-"+panDigits(rng, false), n)
	}
}
