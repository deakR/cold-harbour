package seal

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

var (
	errMissingSigningKey = errors.New("SIGNING_KEY is required")
	errBadSigningKey     = errors.New("SIGNING_KEY must be base64 of a 64-byte ed25519 private key")
	errBadPublicKey      = errors.New("seal: ed25519 public key must be 32 bytes")
)

// PurgePayload is the canonical signed body for a purge receipt.
type PurgePayload struct {
	JobID    string `json:"jobId"`
	PurgedAt string `json:"purgedAt"`
}

// PurgeMessage returns the exact bytes signed for a purge receipt.
// jobID is the Redis id string. purgedAt must already be UTC.
func PurgeMessage(jobID string, purgedAt time.Time) ([]byte, error) {
	return json.Marshal(PurgePayload{
		JobID:    jobID,
		PurgedAt: purgedAt.UTC().Format(time.RFC3339Nano),
	})
}

// SigningKeyID is hex of the first 8 bytes of the Ed25519 public key.
func SigningKeyID(pub ed25519.PublicKey) string {
	if len(pub) < 8 {
		return ""
	}
	return hex.EncodeToString(pub[:8])
}

// Sign returns an Ed25519 signature over msg.
func Sign(priv ed25519.PrivateKey, msg []byte) []byte {
	return ed25519.Sign(priv, msg)
}

// Verify reports whether sig is a valid Ed25519 signature of msg under pub.
func Verify(pub ed25519.PublicKey, msg, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pub, msg, sig)
}

// ParsePrivateKey decodes base64 of a 64-byte ed25519.PrivateKey.
func ParsePrivateKey(b64 string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBadSigningKey, err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, errBadSigningKey
	}
	return ed25519.PrivateKey(raw), nil
}

// ParsePublicKey decodes base64 of a 32-byte ed25519.PublicKey.
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBadPublicKey, err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, errBadPublicKey
	}
	return ed25519.PublicKey(raw), nil
}

// LoadSigningKey reads SIGNING_KEY from the environment.
func LoadSigningKey() (ed25519.PrivateKey, error) {
	b64 := os.Getenv("SIGNING_KEY")
	if b64 == "" {
		return nil, errMissingSigningKey
	}
	return ParsePrivateKey(b64)
}
