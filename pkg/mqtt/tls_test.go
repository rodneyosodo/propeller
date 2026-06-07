package mqtt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTLSConfigFromNil(t *testing.T) {
	cfg, err := tlsConfigFrom(nil)
	if err != nil || cfg != nil {
		t.Fatalf("expected nil,nil got cfg=%v err=%v", cfg, err)
	}
}

func TestTLSConfigFromCertWithoutKey(t *testing.T) {
	_, err := tlsConfigFrom(&TLSConfig{CertPath: "/any", KeyPath: ""})
	if err == nil {
		t.Fatal("expected error for cert-without-key")
	}
}

func TestTLSConfigFromKeyWithoutCert(t *testing.T) {
	_, err := tlsConfigFrom(&TLSConfig{CertPath: "", KeyPath: "/any"})
	if err == nil {
		t.Fatal("expected error for key-without-cert")
	}
}

func TestTLSConfigFromMissingCAFile(t *testing.T) {
	_, err := tlsConfigFrom(&TLSConfig{CAPath: "/nonexistent/ca.crt"})
	if err == nil {
		t.Fatal("expected error for missing CA file")
	}
}

func TestTLSConfigFromInvalidPEM(t *testing.T) {
	f := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(f, []byte("not a pem block"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	_, err := tlsConfigFrom(&TLSConfig{CAPath: f})
	if err == nil {
		t.Fatal("expected error for invalid PEM content")
	}
}

func TestTLSConfigFromInsecureSkipVerify(t *testing.T) {
	cfg, err := tlsConfigFrom(&TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify=true in resulting tls.Config")
	}
}
