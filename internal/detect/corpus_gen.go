package detect

import (
	"bytes"
	"encoding/json"
)

type corpusSpan struct {
	Kind  Kind `json:"kind"`
	Start int  `json:"start"`
	End   int  `json:"end"`
}

type corpusLine struct {
	Text  string       `json:"text"`
	Spans []corpusSpan `json:"spans"`
}

func GenerateCorpus(seed uint64) []byte {
	rng := &lcg{s: seed}
	var lines []corpusLine
	for i := 0; i < 200; i++ {
		lines = append(lines, wrap(formatAadhaar(aadhaarDigits(rng, true), i%3), KindAadhaar))
		lines = append(lines, wrap(formatAadhaar(aadhaarDigits(rng, false), i%3), ""))
		lines = append(lines, wrap(panDigits(rng, true), KindPAN))
		lines = append(lines, wrap(panDigits(rng, false), ""))
		lines = append(lines, wrap(phoneIN(rng, true, i%4), KindPhoneIN))
		lines = append(lines, wrap(phoneIN(rng, false, 0), ""))
		lines = append(lines, wrap(emailAt(i), KindEmail))
		lines = append(lines, wrap(phoneUS(rng, i%3), KindPhoneUS))
		lines = append(lines, wrap(ssnNum(rng), KindSSN))
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

func wrap(value string, kind Kind) corpusLine {
	const prefix = "note "
	text := prefix + value + " end"
	line := corpusLine{Text: text, Spans: []corpusSpan{}}
	if kind != "" {
		line.Spans = []corpusSpan{{Kind: kind, Start: len(prefix), End: len(prefix) + len(value)}}
	}
	return line
}

func aadhaarDigits(rng *lcg, valid bool) string {
	buf := make([]byte, 11)
	buf[0] = rng.from("23456789", 8)
	for i := 1; i < 11; i++ {
		buf[i] = rng.digit()
	}
	check := verhoeffCheck(string(buf))
	if !valid {
		for {
			alt := rng.digit()
			if alt != check {
				check = alt
				break
			}
		}
	}
	return string(buf) + string(check)
}

func formatAadhaar(digits string, form int) string {
	switch form {
	case 1:
		return digits[0:4] + " " + digits[4:8] + " " + digits[8:12]
	case 2:
		return digits[0:4] + "-" + digits[4:8] + "-" + digits[8:12]
	default:
		return digits
	}
}

func panDigits(rng *lcg, valid bool) string {
	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	holders := "ABCFGHLJPT"
	bad := "DEIKMOQRSUVWXYZ"
	pick := func(set string, n uint64) byte { return rng.from(set, n) }
	var buf [10]byte
	buf[0], buf[1], buf[2] = pick(letters, 26), pick(letters, 26), pick(letters, 26)
	if valid {
		buf[3] = pick(holders, 10)
	} else {
		buf[3] = pick(bad, 15)
	}
	buf[4] = pick(letters, 26)
	for i := 5; i < 9; i++ {
		buf[i] = rng.digit()
	}
	buf[9] = pick(letters, 26)
	return string(buf[:])
}

func phoneIN(rng *lcg, valid bool, form int) string {
	var buf [10]byte
	if valid {
		buf[0] = rng.from("6789", 4)
	} else {
		buf[0] = rng.from("12345", 5)
	}
	for i := 1; i < 10; i++ {
		buf[i] = rng.digit()
	}
	d := string(buf[:])
	if !valid {
		return d
	}
	switch form {
	case 1:
		return "+91 " + d[0:5] + " " + d[5:10]
	case 2:
		return "0" + d
	case 3:
		return d[0:5] + "-" + d[5:10]
	default:
		return d
	}
}

func ssnNum(rng *lcg) string {
	digit := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = rng.digit()
		}
		return string(b)
	}
	return digit(3) + "-" + digit(2) + "-" + digit(4)
}

func emailAt(n int) string {
	return "user" + itoa(n) + "@example.com"
}

func phoneUS(rng *lcg, form int) string {
	d := func() byte { return rng.digit() }
	area := []byte{rng.from("2345", 4), d(), d()}
	mid := []byte{d(), d(), d()}
	last := []byte{d(), d(), d(), d()}
	switch form {
	case 1:
		return string(area) + "-" + string(mid) + "-" + string(last)
	case 2:
		return "+1 " + string(area) + " " + string(mid) + " " + string(last)
	default:
		return "(" + string(area) + ") " + string(mid) + "-" + string(last)
	}
}

type lcg struct{ s uint64 }

func (r *lcg) step() uint64 {
	r.s = r.s*6364136223846793005 + 1
	return r.s
}

func (r *lcg) from(set string, n uint64) byte {
	return set[r.step()%n]
}

func (r *lcg) digit() byte {
	return r.from("0123456789", 10)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = "0123456789"[n%10]
		n /= 10
	}
	return string(b[i:])
}
