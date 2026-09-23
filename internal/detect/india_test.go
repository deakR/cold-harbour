package detect

import (
	"strings"
	"testing"
)

func TestAadhaarKnown(t *testing.T) {
	in := "ref 2341 2341 2346 end"
	got := Scan(in, []Kind{KindAadhaar})
	lit := "2341 2341 2346"
	i := strings.Index(in, lit)
	want := []Span{{Kind: KindAadhaar, Start: i, End: i + len(lit)}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("spans = %#v, want %#v", got, want)
	}
	plain := "id 234123412346 ok"
	got = Scan(plain, []Kind{KindAadhaar})
	lit = "234123412346"
	i = strings.Index(plain, lit)
	if len(got) != 1 || got[0].Start != i || got[0].End != i+len(lit) {
		t.Fatalf("plain spans = %#v", got)
	}
	bad := "ref 2341 2341 2340 end"
	if spans := Scan(bad, []Kind{KindAadhaar}); len(spans) != 0 {
		t.Fatalf("bad checksum spans = %#v", spans)
	}
	short := "ref 1234 5678 9012 end"
	if spans := Scan(short, []Kind{KindAadhaar}); len(spans) != 0 {
		t.Fatalf("leading 1 spans = %#v", spans)
	}
}

func TestPANShape(t *testing.T) {
	in := "id ABCPD1234F ok"
	got := Scan(in, []Kind{KindPAN})
	lit := "ABCPD1234F"
	i := strings.Index(in, lit)
	if len(got) != 1 || got[0] != (Span{Kind: KindPAN, Start: i, End: i + len(lit)}) {
		t.Fatalf("spans = %#v", got)
	}
	bad := "id ABCDD1234F ok"
	if spans := Scan(bad, []Kind{KindPAN}); len(spans) != 0 {
		t.Fatalf("bad holder spans = %#v", spans)
	}
}

func TestPhoneIN(t *testing.T) {
	in := "call +91 98765 43210 now"
	got := Scan(in, []Kind{KindPhoneIN})
	lit := "+91 98765 43210"
	i := strings.Index(in, lit)
	if len(got) != 1 || got[0] != (Span{Kind: KindPhoneIN, Start: i, End: i + len(lit)}) {
		t.Fatalf("spans = %#v", got)
	}
	plain := "call 9876543210 now"
	got = Scan(plain, []Kind{KindPhoneIN})
	lit = "9876543210"
	i = strings.Index(plain, lit)
	if len(got) != 1 || got[0].Start != i || got[0].End != i+len(lit) {
		t.Fatalf("plain spans = %#v", got)
	}
	low := "call 1234567890 now"
	if spans := Scan(low, []Kind{KindPhoneIN}); len(spans) != 0 {
		t.Fatalf("leading 1 spans = %#v", spans)
	}
}
