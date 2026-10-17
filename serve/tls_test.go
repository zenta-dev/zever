package serve

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
)

// mustTLSFiles generates a self-signed ECDSA pair into dir, returning the
// cert and key paths.
func mustTLSFiles(t *testing.T, dir string) (certFile, keyFile string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	certOut, err := os.Create(certFile)
	if err != nil {
		t.Fatalf("Create cert: %v", err)
	}

	if pemErr := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); pemErr != nil {
		t.Fatalf("Encode cert: %v", pemErr)
	}

	_ = certOut.Close()

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}

	keyOut, err := os.Create(keyFile)
	if err != nil {
		t.Fatalf("Create key: %v", err)
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatalf("Encode key: %v", err)
	}

	_ = keyOut.Close()

	return certFile, keyFile
}

// TestLoadTLSValidationFailures proves unpaired and missing TLS material
// fails naming the exact variables.
func TestLoadTLSValidationFailures(t *testing.T) {
	cfg := config.Default()

	if _, err := loadTLS(cfg); err != nil {
		t.Fatalf("loadTLS without TLS = %v, want nil", err)
	}

	cfg.Router.Options.TLSCertFile = "cert.pem"
	if _, err := loadTLS(cfg); err == nil {
		t.Fatal("loadTLS cert-without-key = nil, want error")
	} else if got := err.Error(); !containsAll(got, "tls_cert", "tls_key") {
		t.Fatalf("loadTLS err = %q, want cert/key pair named", got)
	}

	cfg.Router.Options.TLSCertFile = filepath.Join(t.TempDir(), "nope.pem")
	cfg.Router.Options.TLSKeyFile = filepath.Join(t.TempDir(), "nope.pem")
	if _, err := loadTLS(cfg); err == nil {
		t.Fatal("loadTLS missing files = nil, want error")
	} else if got := err.Error(); !containsAll(got, "ROUTER_TLS_CERT", "ROUTER_TLS_KEY") {
		t.Fatalf("loadTLS err = %q, want env vars named", got)
	}
}

// TestLoadTLSFloorIsTwelve proves a valid pair builds a TLS 1.2+ config,
// and a bad client CA fails naming its variable.
func TestLoadTLSFloorIsTwelve(t *testing.T) {
	dir := t.TempDir()
	certFile, keyFile := mustTLSFiles(t, dir)

	cfg := config.Default()
	cfg.Router.Options.TLSCertFile = certFile
	cfg.Router.Options.TLSKeyFile = keyFile

	tlsCfg, err := loadTLS(cfg)
	if err != nil {
		t.Fatalf("loadTLS = %v, want nil", err)
	}

	if tlsCfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %x, want TLS 1.2", tlsCfg.MinVersion)
	}

	badCA := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(badCA, []byte("not-pem"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg.Router.Options.TLSClientCAFile = badCA
	if _, err := loadTLS(cfg); err == nil {
		t.Fatal("loadTLS bad client CA = nil, want error")
	} else if got := err.Error(); !containsAll(got, "ROUTER_TLS_CLIENT_CA") {
		t.Fatalf("loadTLS err = %q, want client CA var named", got)
	}
}

// TestReflectionEnabledMatrix proves reflection is off by default, on in
// dev, and explicitly gated by GRPC_REFLECTION.
func TestReflectionEnabledMatrix(t *testing.T) {
	cases := []struct {
		name     string
		grpcRefl string
		hasRefl  bool
		zeverEnv string
		hasEnv   bool
		want     bool
	}{
		{name: "default off", want: false},
		{name: "dev on", zeverEnv: "dev", hasEnv: true, want: true},
		{name: "prod off", zeverEnv: "production", hasEnv: true, want: false},
		{name: "explicit on", grpcRefl: "true", hasRefl: true, want: true},
		{name: "explicit off beats dev", grpcRefl: "false", hasRefl: true, zeverEnv: "dev", hasEnv: true, want: false},
		{name: "garbage fails closed", grpcRefl: "maybe", hasRefl: true, zeverEnv: "dev", hasEnv: true, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.hasRefl {
				t.Setenv("GRPC_REFLECTION", tc.grpcRefl)
			} else {
				unsetEnv(t, "GRPC_REFLECTION")
			}

			if tc.hasEnv {
				t.Setenv("ZEVER_ENV", tc.zeverEnv)
			} else {
				unsetEnv(t, "ZEVER_ENV")
			}

			if got := reflectionEnabled(); got != tc.want {
				t.Fatalf("reflectionEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

// unsetEnv removes key for the test, restoring the prior value after.
func unsetEnv(t *testing.T, key string) {
	t.Helper()

	old, had := lookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("Unsetenv(%s): %v", key, err)
	}

	t.Cleanup(func() {
		if had {
			//nolint:usetesting // t.Setenv cannot run inside Cleanup; restore directly.
			_ = os.Setenv(key, old)
		}
	})
}

// TestIsLoopbackAddrMatrix proves loopback detection for the plaintext
// warning.
func TestIsLoopbackAddrMatrix(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8080":   true,
		"[::1]:8080":       true,
		"localhost:8080":   true,
		":8080":            false,
		"0.0.0.0:8080":     false,
		"example.com:8080": false,
		"not-an-addr":      false,
	}

	for addr, want := range cases {
		if got := isLoopbackAddr(addr); got != want {
			t.Errorf("isLoopbackAddr(%q) = %v, want %v", addr, got, want)
		}
	}
}

// containsAll reports whether s contains every sub.
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}

	return true
}
