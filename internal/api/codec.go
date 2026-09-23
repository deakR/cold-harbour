package api

import (
	"encoding/base64"
	"encoding/json"
)

func b64(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func jsonRaw(body string) any {
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return body
	}
	return v
}
