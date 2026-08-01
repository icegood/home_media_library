package gatewayconfig

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTransportValidation(t *testing.T) {
	if err := Validate(Settings{}); err == nil {
		t.Fatal("both protocols disabled must be rejected")
	}
	if err := Validate(Settings{HTTPEnabled: true}); err != nil {
		t.Fatalf("default HTTP settings rejected: %v", err)
	}
	if err := Validate(Settings{HTTPSEnabled: true}); err == nil {
		t.Fatal("HTTPS without public DNS and email must be rejected")
	}
	if err := Validate(Settings{
		HTTPSEnabled: true, PublicDNS: "media.example.com", ACMEEmail: "admin@example.com",
	}); err != nil {
		t.Fatalf("valid HTTPS settings rejected: %v", err)
	}
}

func TestWriteGeneratesSelectedListeners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Caddyfile")
	settings := Settings{
		HTTPEnabled: true, HTTPSEnabled: true,
		PublicDNS: "media.example.com", ACMEEmail: "admin@example.com",
	}
	if err := Write(path, settings); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	config := string(data)
	for _, expected := range []string{"http://:80", "media.example.com", letsEncryptCA} {
		if !strings.Contains(config, expected) {
			t.Fatalf("generated config does not contain %q:\n%s", expected, config)
		}
	}
}

func TestCertificateExpirationReadsCaddyCertificate(t *testing.T) {
	dataDir := t.TempDir()
	certDir := filepath.Join(dataDir, "caddy", "certificates", "acme-v02.api.letsencrypt.org-directory", "media.example.com")
	if err := os.MkdirAll(certDir, 0o700); err != nil {
		t.Fatal(err)
	}
	expires := time.Date(2026, 12, 25, 12, 0, 0, 0, time.UTC)
	writeTestCertificate(t, filepath.Join(certDir, "media.example.com.crt"), expires)
	got, ok := CertificateExpiration(dataDir, "media.example.com")
	if !ok {
		t.Fatal("certificate expiration was not found")
	}
	if !got.Equal(expires) {
		t.Fatalf("expiration = %s, want %s", got, expires)
	}
}

func writeTestCertificate(t *testing.T, path string, expires time.Time) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "media.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     expires,
		DNSNames:     []string{"media.example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
