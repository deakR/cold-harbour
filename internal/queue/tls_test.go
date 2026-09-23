package queue

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRedisPingRejectsWrongCA(t *testing.T) {
	caKey, caCert := newCA(t)
	serverCert := signServer(t, caKey, caCert)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{serverCert}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1)
				_, _ = c.Read(buf)
			}(conn)
		}
	}()

	dir := t.TempDir()
	right := filepath.Join(dir, "right.crt")
	wrong := filepath.Join(dir, "wrong.crt")
	if err := os.WriteFile(right, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	_, other := newCA(t)
	if err := os.WriteFile(wrong, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: other.Raw}), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("REDIS_TLS", "1")
	t.Setenv("REDIS_PASSWORD", "")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	t.Setenv("REDIS_CA", wrong)
	bad := NewRedisClient(ln.Addr().String())
	t.Cleanup(func() { _ = bad.Close() })
	err = bad.Ping(ctx).Err()
	if err == nil || !strings.Contains(err.Error(), "unknown authority") {
		t.Fatalf("wrong CA ping err = %v, want unknown authority", err)
	}

	t.Setenv("REDIS_CA", right)
	good := NewRedisClient(ln.Addr().String())
	t.Cleanup(func() { _ = good.Close() })
	err = good.Ping(ctx).Err()
	if err != nil && strings.Contains(err.Error(), "unknown authority") {
		t.Fatalf("right CA was rejected: %v", err)
	}
}

func newCA(t *testing.T) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return key, cert
}

func signServer(t *testing.T, caKey *ecdsa.PrivateKey, ca *x509.Certificate) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "redis"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"redis", "localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	return pair
}
