package policy

import (
	"bytes"
	"encoding/json"

	"coldharbour/internal/detect"
)

type Doc struct {
	Version        int           `json:"version"`
	Detectors      []detect.Kind `json:"detectors"`
	Mode           string        `json:"mode"`
	FailPolicy     string        `json:"failPolicy"`
	MaxInlineBytes int           `json:"maxInlineBytes"`
}

type FieldError struct {
	Field string
}

func (e *FieldError) Error() string { return e.Field }

func Default() Doc {
	return Doc{
		Version:        0,
		Detectors:      []detect.Kind{detect.KindEmail, detect.KindPhoneUS, detect.KindSSN},
		Mode:           string(detect.ModeRedact),
		FailPolicy:     "closed",
		MaxInlineBytes: 65536,
	}
}

var knownKinds = map[detect.Kind]struct{}{
	detect.KindEmail:   {},
	detect.KindPhoneUS: {},
	detect.KindPhoneIN: {},
	detect.KindSSN:     {},
	detect.KindAadhaar: {},
	detect.KindPAN:     {},
}

func Parse(raw []byte) (Doc, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var doc Doc
	if err := dec.Decode(&doc); err != nil {
		return Doc{}, &FieldError{Field: "detectors"}
	}
	if len(doc.Detectors) == 0 {
		return Doc{}, &FieldError{Field: "detectors"}
	}
	for _, kind := range doc.Detectors {
		if _, ok := knownKinds[kind]; !ok {
			return Doc{}, &FieldError{Field: "detectors"}
		}
	}
	if doc.Mode != string(detect.ModeRedact) && doc.Mode != string(detect.ModePartial) {
		return Doc{}, &FieldError{Field: "mode"}
	}
	if doc.FailPolicy != "closed" && doc.FailPolicy != "inline" && doc.FailPolicy != "open" {
		return Doc{}, &FieldError{Field: "failPolicy"}
	}
	if doc.MaxInlineBytes < 1 {
		return Doc{}, &FieldError{Field: "maxInlineBytes"}
	}
	return doc, nil
}
