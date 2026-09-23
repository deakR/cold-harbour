package detect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestCorpusAccuracy(t *testing.T) {
	f, err := os.Open("testdata/corpus.jsonl")
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	defer f.Close()
	var lines []corpusLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var line corpusLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			t.Fatalf("json: %v", err)
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	kinds := []Kind{KindEmail, KindPhoneUS, KindPhoneIN, KindSSN, KindAadhaar, KindPAN}
	type stat struct{ pos, neg, tp, fp, fn int }
	stats := map[Kind]*stat{}
	for _, kind := range kinds {
		stats[kind] = &stat{}
	}
	for _, line := range lines {
		got := Scan(line.Text, kinds)
		labeled := map[Span]struct{}{}
		present := map[Kind]bool{}
		for _, sp := range line.Spans {
			labeled[Span{Kind: sp.Kind, Start: sp.Start, End: sp.End}] = struct{}{}
			present[sp.Kind] = true
		}
		for _, kind := range kinds {
			if present[kind] {
				stats[kind].pos++
			} else {
				stats[kind].neg++
			}
		}
		found := map[Span]struct{}{}
		for _, sp := range got {
			found[sp] = struct{}{}
			if _, ok := labeled[sp]; ok {
				stats[sp.Kind].tp++
			} else {
				stats[sp.Kind].fp++
			}
		}
		for sp := range labeled {
			if _, ok := found[sp]; !ok {
				stats[sp.Kind].fn++
			}
		}
	}
	for _, kind := range kinds {
		st := stats[kind]
		p := 1.0
		if st.tp+st.fp > 0 {
			p = float64(st.tp) / float64(st.tp+st.fp)
		}
		r := 1.0
		if st.tp+st.fn > 0 {
			r = float64(st.tp) / float64(st.tp+st.fn)
		}
		fmt.Printf("%s precision %.4f recall %.4f\n", kind, p, r)
	}
	for _, kind := range []Kind{KindAadhaar, KindPAN, KindPhoneIN} {
		st := stats[kind]
		if st.pos < 200 || st.neg < 200 {
			t.Fatalf("%s lines pos=%d neg=%d, want at least 200 of each", kind, st.pos, st.neg)
		}
	}
	for _, kind := range []Kind{KindAadhaar, KindPAN} {
		st := stats[kind]
		if st.fn != 0 {
			t.Fatalf("%s false negatives = %d, want 0", kind, st.fn)
		}
		if st.tp*200 < 199*(st.tp+st.fp) {
			t.Fatalf("%s precision below 99.5%% tp=%d fp=%d", kind, st.tp, st.fp)
		}
	}
	for _, kind := range []Kind{KindPhoneIN, KindPhoneUS, KindEmail} {
		st := stats[kind]
		if st.tp+st.fp == 0 {
			t.Fatalf("%s has no detections", kind)
		}
		if st.tp*100 < 98*(st.tp+st.fp) {
			t.Fatalf("%s precision below 98%% tp=%d fp=%d", kind, st.tp, st.fp)
		}
	}
}

// TestRealisticAccuracy grades Scan on testdata/realistic.jsonl.
// On a miss, triage by kind and byte offsets only, then either fix the matcher
// in a later PR or relabel. Do not lower a floor to pass.
func TestRealisticAccuracy(t *testing.T) {
	f, err := os.Open("testdata/realistic.jsonl")
	if err != nil {
		t.Fatalf("open realistic corpus: %v", err)
	}
	defer f.Close()
	var lines []corpusLine
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var line corpusLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			t.Fatalf("json: %v", err)
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	kinds := []Kind{KindEmail, KindPhoneUS, KindPhoneIN, KindSSN, KindAadhaar, KindPAN}
	type floor struct{ p, r float64 }
	floors := map[Kind]floor{
		KindAadhaar: {0.995, 0.990},
		KindPAN:     {0.995, 0.990},
		KindEmail:   {0.980, 0.950},
		KindPhoneUS: {0.980, 0.950},
		KindPhoneIN: {0.980, 0.950},
		KindSSN:     {0.980, 0.950},
	}
	type stat struct{ pos, neg, tp, fp, fn int }
	stats := map[Kind]*stat{}
	for _, kind := range kinds {
		stats[kind] = &stat{}
	}
	emptyLines := 0
	for i, line := range lines {
		got := Scan(line.Text, kinds)
		labeled := map[Span]struct{}{}
		present := map[Kind]bool{}
		for _, sp := range line.Spans {
			labeled[Span{Kind: sp.Kind, Start: sp.Start, End: sp.End}] = struct{}{}
			present[sp.Kind] = true
		}
		if len(line.Spans) == 0 {
			emptyLines++
		}
		for _, kind := range kinds {
			if present[kind] {
				stats[kind].pos++
			} else {
				stats[kind].neg++
			}
		}
		found := map[Span]struct{}{}
		for _, sp := range got {
			found[sp] = struct{}{}
			if _, ok := labeled[sp]; ok {
				stats[sp.Kind].tp++
			} else {
				stats[sp.Kind].fp++
				t.Errorf("line %d fp kind=%s start=%d end=%d", i, sp.Kind, sp.Start, sp.End)
			}
		}
		for sp := range labeled {
			if _, ok := found[sp]; !ok {
				stats[sp.Kind].fn++
				t.Errorf("line %d fn kind=%s start=%d end=%d", i, sp.Kind, sp.Start, sp.End)
			}
		}
	}
	for _, kind := range kinds {
		st := stats[kind]
		p := 1.0
		if st.tp+st.fp > 0 {
			p = float64(st.tp) / float64(st.tp+st.fp)
		}
		r := 1.0
		if st.tp+st.fn > 0 {
			r = float64(st.tp) / float64(st.tp+st.fn)
		}
		fmt.Printf("%s precision %.4f recall %.4f\n", kind, p, r)
		if st.pos < 100 {
			t.Fatalf("%s positive lines=%d, want at least 100", kind, st.pos)
		}
		fl := floors[kind]
		if p < fl.p {
			t.Fatalf("%s precision %.4f below floor %.3f tp=%d fp=%d", kind, p, fl.p, st.tp, st.fp)
		}
		if r < fl.r {
			t.Fatalf("%s recall %.4f below floor %.3f tp=%d fn=%d", kind, r, fl.r, st.tp, st.fn)
		}
	}
	if emptyLines < 300 {
		t.Fatalf("near-miss lines with no spans=%d, want at least 300", emptyLines)
	}
}
