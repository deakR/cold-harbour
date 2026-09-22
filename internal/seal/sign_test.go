package seal

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"
)

func TestSignVerifyPurgeMessage(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 15, 12, 0, 0, 123456789, time.UTC)
	msg, err := PurgeMessage("redis-job-1", at)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"jobId":"redis-job-1","purgedAt":"2026-03-15T12:00:00.123456789Z"}`
	if string(msg) != want {
		t.Fatalf("PurgeMessage = %s, want %s", msg, want)
	}
	sig := Sign(priv, msg)
	if !Verify(pub, msg, sig) {
		t.Fatal("Verify failed for valid signature")
	}
	msg[len(msg)-2] ^= 0xff
	if Verify(pub, msg, sig) {
		t.Fatal("Verify succeeded after payload flip")
	}
}

func TestSigningKeyIDAndParse(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := SigningKeyID(pub)
	if len(id) != 16 {
		t.Fatalf("SigningKeyID len = %d, want 16", len(id))
	}
	b64 := base64.StdEncoding.EncodeToString(priv)
	got, err := ParsePrivateKey(b64)
	if err != nil {
		t.Fatal(err)
	}
	if !Verify(pub, []byte("x"), Sign(got, []byte("x"))) {
		t.Fatal("parsed private key does not match")
	}
}
