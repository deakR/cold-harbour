package seal

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

func vaultOn() bool {
	return os.Getenv("VAULT_ADDR") != "" && os.Getenv("VAULT_TOKEN") != ""
}

func wrapKey(ctx context.Context, plain []byte) ([]byte, error) {
	if !vaultOn() {
		return plain, nil
	}
	body, _ := json.Marshal(map[string]string{
		"plaintext": base64.StdEncoding.EncodeToString(plain),
	})
	raw, err := vaultPost(ctx, "/v1/transit/encrypt/coldharbour", body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Data struct {
			Ciphertext string `json:"ciphertext"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	if parsed.Data.Ciphertext == "" {
		return nil, fmt.Errorf("vault encrypt returned an empty ciphertext")
	}
	return []byte(parsed.Data.Ciphertext), nil
}

func unwrapKey(ctx context.Context, stored []byte) ([]byte, error) {
	if !vaultOn() {
		if len(stored) != 32 {
			return nil, fmt.Errorf("%w: got %d", errKeyWrongLength, len(stored))
		}
		return stored, nil
	}
	body, _ := json.Marshal(map[string]string{"ciphertext": string(stored)})
	raw, err := vaultPost(ctx, "/v1/transit/decrypt/coldharbour", body)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Data struct {
			Plaintext string `json:"plaintext"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	plain, err := base64.StdEncoding.DecodeString(parsed.Data.Plaintext)
	if err != nil {
		return nil, err
	}
	if len(plain) != 32 {
		return nil, fmt.Errorf("%w: got %d", errKeyWrongLength, len(plain))
	}
	return plain, nil
}

func vaultPost(ctx context.Context, path string, body []byte) ([]byte, error) {
	// VAULT_ADDR is set by the operator, not by a request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, os.Getenv("VAULT_ADDR")+path, bytes.NewReader(body)) //#nosec G704
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", os.Getenv("VAULT_TOKEN"))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req) //#nosec G704
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("vault %s: %s", path, raw)
	}
	return raw, nil
}
