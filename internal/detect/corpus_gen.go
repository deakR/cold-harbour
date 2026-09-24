package detect

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"strconv"
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
	rng := rand.New(rand.NewPCG(seed, seed)) //#nosec G404 -- fixed seed for fixtures, not a secret
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

func aadhaarDigits(rng *rand.Rand, valid bool) string {
	buf := make([]byte, 11)
	buf[0] = pick(rng, "23456789")
	for i := 1; i < 11; i++ {
		buf[i] = pick(rng, "0123456789")
	}
	check := verhoeffCheck(string(buf))
	if !valid {
		for {
			alt := pick(rng, "0123456789")
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

func panDigits(rng *rand.Rand, valid bool) string {
	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var buf [10]byte
	buf[0], buf[1], buf[2] = pick(rng, letters), pick(rng, letters), pick(rng, letters)
	if valid {
		buf[3] = pick(rng, "ABCFGHLJPT")
	} else {
		buf[3] = pick(rng, "DEIKMOQRSUVWXYZ")
	}
	buf[4] = pick(rng, letters)
	for i := 5; i < 9; i++ {
		buf[i] = pick(rng, "0123456789")
	}
	buf[9] = pick(rng, letters)
	return string(buf[:])
}

func phoneIN(rng *rand.Rand, valid bool, form int) string {
	var buf [10]byte
	if valid {
		buf[0] = pick(rng, "6789")
	} else {
		buf[0] = pick(rng, "12345")
	}
	for i := 1; i < 10; i++ {
		buf[i] = pick(rng, "0123456789")
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

func ssnNum(rng *rand.Rand) string {
	digit := func(n int) string {
		b := make([]byte, n)
		for i := range b {
			b[i] = pick(rng, "0123456789")
		}
		return string(b)
	}
	return digit(3) + "-" + digit(2) + "-" + digit(4)
}

func emailAt(n int) string {
	return "user" + strconv.Itoa(n) + "@example.com"
}

func phoneUS(rng *rand.Rand, form int) string {
	d := func() byte { return pick(rng, "0123456789") }
	area := []byte{pick(rng, "2345"), d(), d()}
	mid := []byte{d(), d(), d()}
	last := []byte{d(), d(), d(), d()}
	switch form {
	case 1:
		return string(area) + "-" + string(mid) + "-" + string(last)
	case 2:
		return "+1 " + string(area) + " " + string(mid) + " " + string(last)
	case 3:
		return string(area) + " " + string(mid) + " " + string(last)
	default:
		return "(" + string(area) + ") " + string(mid) + "-" + string(last)
	}
}

func pick(rng *rand.Rand, set string) byte {
	return set[rng.IntN(len(set))]
}
