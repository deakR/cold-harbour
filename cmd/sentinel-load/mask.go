package main

import "coldharbour/internal/detect"

var maskKinds = []detect.Kind{
	detect.KindEmail,
	detect.KindPhoneUS,
	detect.KindPhoneIN,
	detect.KindSSN,
	detect.KindAadhaar,
	detect.KindPAN,
}

func maskLine(line string) string {
	spans := detect.Scan(line, maskKinds)
	out, _ := detect.Apply(line, spans, detect.ModeRedact)
	return out
}
