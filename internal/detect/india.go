package detect

import "regexp"

var (
	aadhaarRE = regexp.MustCompile(`[2-9]\d{3}[ -]?\d{4}[ -]?\d{4}`)
	panRE     = regexp.MustCompile(`\b[A-Z]{3}[ABCFGHLJPT][A-Z][0-9]{4}[A-Z]\b`)
	phoneINRE = regexp.MustCompile(`(?:\+91[ -]?|0)?[6-9](?:[ -]?\d){9}`)
)

func findAadhaar(s string) []Span {
	var out []Span
	for _, loc := range aadhaarRE.FindAllStringIndex(s, -1) {
		if loc[0] > 0 && isDigit(s[loc[0]-1]) {
			continue
		}
		if loc[1] < len(s) && isDigit(s[loc[1]]) {
			continue
		}
		digits := digitsOnly(s[loc[0]:loc[1]])
		if len(digits) != 12 || !verhoeffOK(digits) {
			continue
		}
		out = append(out, Span{Kind: KindAadhaar, Start: loc[0], End: loc[1]})
	}
	return out
}

func findPAN(s string) []Span {
	locs := panRE.FindAllStringIndex(s, -1)
	out := make([]Span, 0, len(locs))
	for _, loc := range locs {
		out = append(out, Span{Kind: KindPAN, Start: loc[0], End: loc[1]})
	}
	return out
}

func findPhoneIN(s string) []Span {
	var out []Span
	for _, loc := range phoneINRE.FindAllStringIndex(s, -1) {
		if loc[1] < len(s) && isDigit(s[loc[1]]) {
			continue
		}
		if s[loc[0]] != '+' && loc[0] > 0 && isDigit(s[loc[0]-1]) {
			continue
		}
		out = append(out, Span{Kind: KindPhoneIN, Start: loc[0], End: loc[1]})
	}
	return out
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func digitsOnly(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if isDigit(s[i]) {
			out = append(out, s[i])
		}
	}
	return string(out)
}

var verhoeffD = [10][10]int{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
	{1, 2, 3, 4, 0, 6, 7, 8, 9, 5},
	{2, 3, 4, 0, 1, 7, 8, 9, 5, 6},
	{3, 4, 0, 1, 2, 8, 9, 5, 6, 7},
	{4, 0, 1, 2, 3, 9, 5, 6, 7, 8},
	{5, 9, 8, 7, 6, 0, 4, 3, 2, 1},
	{6, 5, 9, 8, 7, 1, 0, 4, 3, 2},
	{7, 6, 5, 9, 8, 2, 1, 0, 4, 3},
	{8, 7, 6, 5, 9, 3, 2, 1, 0, 4},
	{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
}

var verhoeffP = [8][10]int{
	{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
	{1, 5, 7, 6, 2, 8, 3, 0, 9, 4},
	{5, 8, 0, 3, 7, 9, 6, 1, 4, 2},
	{8, 9, 1, 6, 0, 4, 3, 5, 2, 7},
	{9, 4, 5, 3, 1, 2, 6, 8, 7, 0},
	{4, 2, 8, 6, 5, 7, 3, 9, 0, 1},
	{2, 7, 9, 3, 8, 0, 6, 4, 1, 5},
	{7, 0, 4, 6, 9, 1, 3, 2, 5, 8},
}

var verhoeffInv = [10]int{0, 4, 3, 2, 1, 5, 6, 7, 8, 9}

func verhoeffOK(digits string) bool {
	c := 0
	for i, j := len(digits)-1, 0; i >= 0; i, j = i-1, j+1 {
		c = verhoeffD[c][verhoeffP[j%8][digits[i]-'0']]
	}
	return c == 0
}

func verhoeffCheck(payload string) byte {
	c := 0
	for i, j := len(payload)-1, 1; i >= 0; i, j = i-1, j+1 {
		c = verhoeffD[c][verhoeffP[j%8][payload[i]-'0']]
	}
	return "0123456789"[verhoeffInv[c]]
}
