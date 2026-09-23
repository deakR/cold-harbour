package detect

import (
	"reflect"
	"strings"
	"testing"
)

const fixture = `Contact jane.doe@example.com or (555) 123-4567 for details.
SSN on file: 123-45-6789. Backup contact: john@company.org.
Not a match: version 123-45 or year 1234-56-789.`

func at(s, lit string, kind Kind) Span {
	i := strings.Index(s, lit)
	if i < 0 {
		panic("missing " + lit)
	}
	return Span{Kind: kind, Start: i, End: i + len(lit)}
}

func TestScanFixture(t *testing.T) {
	got := Scan(fixture, []Kind{KindEmail, KindPhoneUS, KindSSN})
	want := []Span{
		at(fixture, "jane.doe@example.com", KindEmail),
		at(fixture, "(555) 123-4567", KindPhoneUS),
		at(fixture, "123-45-6789", KindSSN),
		at(fixture, "john@company.org", KindEmail),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans = %#v, want %#v", got, want)
	}
}

func TestLongerSpanWins(t *testing.T) {
	in := "123-456-7890@x.com"
	got := Scan(in, []Kind{KindEmail, KindPhoneUS})
	want := []Span{at(in, "123-456-7890", KindPhoneUS)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans = %#v, want %#v", got, want)
	}
}

func TestPercentEncodedQueryEmail(t *testing.T) {
	in := `GET /v1/lookup?email=user%40example.com&page=1 HTTP/1.1`
	got := Scan(in, []Kind{KindEmail})
	want := []Span{{Kind: KindEmail, Start: 21, End: 39}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans = %#v, want %#v", got, want)
	}
}

func TestSyntheticEmailStillMatches(t *testing.T) {
	in := "user0@example.com " + strings.Repeat("x", 100)
	got := Scan(in, []Kind{KindEmail})
	want := []Span{at(in, "user0@example.com", KindEmail)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans = %#v, want %#v", got, want)
	}
	out, _ := Apply(in, got, ModeRedact)
	if strings.Contains(out, "user0@example.com") {
		t.Fatalf("raw email remained in %q", out)
	}
	if !strings.Contains(out, "[EMAIL]") {
		t.Fatalf("missing [EMAIL] placeholder in %q", out)
	}
}

func TestSpaceSeparatedUSPhone(t *testing.T) {
	in := `Contact 415 555 0199 today`
	got := Scan(in, []Kind{KindPhoneUS})
	want := []Span{{Kind: KindPhoneUS, Start: 8, End: 20}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("spans = %#v, want %#v", got, want)
	}
}

func TestApplyRedactAndPartial(t *testing.T) {
	in := "xx12345678yy"
	spans := []Span{{Kind: KindSSN, Start: 2, End: 10}}
	got, counts := Apply(in, spans, ModeRedact)
	if got != "xx[SSN]yy" {
		t.Fatalf("redact = %q", got)
	}
	if counts[KindSSN] != 1 || len(counts) != 1 {
		t.Fatalf("counts = %#v", counts)
	}
	got, counts = Apply(in, spans, ModePartial)
	if got != "xx****5678yy" {
		t.Fatalf("partial = %q", got)
	}
	if counts[KindSSN] != 1 {
		t.Fatalf("partial counts = %#v", counts)
	}
}
