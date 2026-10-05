package detect

import (
	"regexp"
)

const (
	KindCreditCard Kind = "credit_card"
	KindIP         Kind = "ip"
)

var (
	// Matches standard 13-19 digit card patterns with optional spaces or dashes
	cardRE = regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`)
)

func findCreditCard(s string) []Span {
	var out []Span
	for _, loc := range cardRE.FindAllStringIndex(s, -1) {
		if loc[0] > 0 && isDigit(s[loc[0]-1]) {
			continue
		}
		if loc[1] < len(s) && isDigit(s[loc[1]]) {
			continue
		}
		raw := s[loc[0]:loc[1]]
		digits := digitsOnly(raw)
		if len(digits) < 13 || len(digits) > 19 {
			continue
		}
		// Basic IIN / Major Industry Identifier sanity check:
		// 3 = Amex/Diners, 4 = Visa, 5 = Mastercard, 6 = Discover/RuPay
		first := digits[0]
		if first < '3' || first > '6' {
			continue
		}
		if !luhnOK(digits) {
			continue
		}
		out = append(out, Span{Kind: KindCreditCard, Start: loc[0], End: loc[1]})
	}
	return out
}

func luhnOK(digits string) bool {
	sum := 0
	alternate := false
	for i := len(digits) - 1; i >= 0; i-- {
		n := int(digits[i] - '0')
		if n < 0 || n > 9 {
			return false
		}
		if alternate {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alternate = !alternate
	}
	return sum%10 == 0
}
