package mqtt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/absmach/propeller/pkg/mqtt"
)

func TestTLSConfigFromNil(t *testing.T) {
	t.Parallel()

	cfg, err := mqtt.TLSConfigFrom(nil)
	if err != nil || cfg != nil {
		t.Fatalf("expected nil,nil got cfg=%v err=%v", cfg, err)
	}
}

func TestTLSConfigFromCertWithoutKey(t *testing.T) {
	t.Parallel()

	_, err := mqtt.TLSConfigFrom(&mqtt.TLSConfig{CertPath: "/any", KeyPath: ""})
	if err == nil {
		t.Fatal("expected error for cert-without-key")
	}
}

func TestTLSConfigFromKeyWithoutCert(t *testing.T) {
	t.Parallel()

	_, err := mqtt.TLSConfigFrom(&mqtt.TLSConfig{CertPath: "", KeyPath: "/any"})
	if err == nil {
		t.Fatal("expected error for key-without-cert")
	}
}

func TestTLSConfigFromMissingCAFile(t *testing.T) {
	t.Parallel()

	_, err := mqtt.TLSConfigFrom(&mqtt.TLSConfig{CAPath: "/nonexistent/ca.crt"})
	if err == nil {
		t.Fatal("expected error for missing CA file")
	}
}

func TestTLSConfigFromInvalidPEM(t *testing.T) {
	t.Parallel()

	f := filepath.Join(t.TempDir(), "bad.pem")
	if err := os.WriteFile(f, []byte("not a pem block"), 0o600); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err := mqtt.TLSConfigFrom(&mqtt.TLSConfig{CAPath: f})
	if err == nil {
		t.Fatal("expected error for invalid PEM content")
	}
}

func TestTLSConfigFromInsecureSkipVerify(t *testing.T) {
	t.Parallel()

	cfg, err := mqtt.TLSConfigFrom(&mqtt.TLSConfig{InsecureSkipVerify: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cfg.InsecureSkipVerify {
		t.Fatal("expected InsecureSkipVerify=true in resulting tls.Config")
	}
}
