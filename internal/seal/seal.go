package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const nonceSize = 12

var errBadSealed = errors.New("seal: invalid sealed payload")

// Seal returns standard base64 of a 12-byte nonce concatenated with AES-GCM ciphertext.
func Seal(key [32]byte, plain []byte) (string, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := gcm.Seal(nonce, nonce, plain, nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Open reverses Seal: decode base64, split nonce || ciphertext, AES-GCM open.
func Open(key [32]byte, sealed string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBadSealed, err)
	}
	if len(raw) <= nonceSize {
		return nil, errBadSealed
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errBadSealed, err)
	}
	return plain, nil
}
