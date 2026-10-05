package detect

import (
	"net"
	"regexp"
	"strings"
)

var (
	ipv4RE = regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`)
	ipv6RE = regexp.MustCompile(`(?:(?:\b[0-9a-fA-F]{1,4}:)|::)[0-9a-fA-F:]*\b|::1`)
)

func findIP(s string) []Span {
	var out []Span
	for _, loc := range ipv4RE.FindAllStringIndex(s, -1) {
		candidate := s[loc[0]:loc[1]]
		ip := net.ParseIP(candidate)
		if ip != nil && ip.To4() != nil {
			out = append(out, Span{Kind: KindIP, Start: loc[0], End: loc[1]})
		}
	}
	for _, loc := range ipv6RE.FindAllStringIndex(s, -1) {
		candidate := s[loc[0]:loc[1]]
		if !strings.Contains(candidate, ":") {
			continue
		}
		ip := net.ParseIP(candidate)
		if ip != nil && ip.To4() == nil {
			out = append(out, Span{Kind: KindIP, Start: loc[0], End: loc[1]})
		}
	}
	return out
}
