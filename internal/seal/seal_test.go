package seal

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestSealOpenRoundTrip(t *testing.T) {
	var key [32]byte
	copy(key[:], []byte("0123456789abcdef0123456789abcdef"))
	plain := []byte(`{"redactedText":"x"}`)
	sealed, err := Seal(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	if json.Valid([]byte(sealed)) {
		t.Fatalf("Seal produced JSON: %s", sealed)
	}
	got, err := Open(key, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(plain) {
		t.Fatalf("Open = %q, want %q", got, plain)
	}
}

func TestSealKnownAnswer(t *testing.T) {
	var key [32]byte
	for i := range key {
		key[i] = 1
	}
	got, err := sealWithNonce(key, bytes.Repeat([]byte{2}, 12), []byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	const want = "AgICAgICAgICAgICb7OlJSUCYwtZ7fA+dwycA4aL9zrt"
	if got != want {
		t.Fatalf("Seal = %s, want %s", got, want)
	}
	plain, err := Open(key, got)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != "hello" {
		t.Fatalf("Open = %q, want hello", plain)
	}
}

func TestOpenRejectsJSON(t *testing.T) {
	var key [32]byte
	copy(key[:], []byte("0123456789abcdef0123456789abcdef"))
	_, err := Open(key, `{"redactedText":"x"}`)
	if err == nil {
		t.Fatal("Open(JSON) err = nil, want error")
	}
}

func TestMemoryKeyStoreEnsureOnce(t *testing.T) {
	store := NewMemoryKeyStore()
	id := uuid.New()
	a, err := store.Ensure(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.Ensure(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("Ensure returned a second key for the same id")
	}
}
