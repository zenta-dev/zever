package observability

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSealers_noop(t *testing.T) {
	t.Parallel()

	StringValue{}.isAttributeValue()
	Int64Value{}.isAttributeValue()
	Float64Value{}.isAttributeValue()
	BoolValue{}.isAttributeValue()
}

func TestNormalizeAttrs_keyTruncation_cutsLongKeys(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("k", MaxKeyLen+44)
	got := normalizeAttrs([]Attr{String(long, "v")}, 0)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if len(got[0].Key) != MaxKeyLen {
		t.Errorf("key len = %d, want %d", len(got[0].Key), MaxKeyLen)
	}
}

func TestOptionsValidate_unreadableCAFile(t *testing.T) {
	t.Parallel()

	opts := validOptions()
	opts.CAFile = "/nonexistent/ca.pem"

	err := opts.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want unreadable CA error")
	}
	if !errors.Is(err, ErrInvalidOptions) {
		t.Errorf("errors.Is(err, ErrInvalidOptions) = false (err = %v)", err)
	}
}

func TestOptionsValidate_invalidCAPEM(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, []byte("not-pem"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	opts := validOptions()
	opts.CAFile = path

	if err := opts.Validate(); err == nil {
		t.Fatal("Validate() = nil, want invalid CA contents error")
	}
}

func TestOptionsValidate_validCAFile_passes(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	fh, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := pem.Encode(fh, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		_ = fh.Close()
		t.Fatalf("Encode() error = %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	opts := validOptions()
	opts.CAFile = path

	if err := opts.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestOptionsValidate_emptyEndpointHost(t *testing.T) {
	t.Parallel()

	opts := validOptions()
	opts.Endpoint = ":4317"

	if err := opts.Validate(); err == nil {
		t.Fatal("Validate() = nil, want empty host error")
	}
}

func TestOptionsValidate_headerTokenShapes(t *testing.T) {
	t.Parallel()

	single := validOptions()
	single.Headers = map[string]string{"a": "v"}
	if err := single.Validate(); err != nil {
		t.Errorf("Validate() single-char header error = %v, want nil", err)
	}

	special := validOptions()
	special.Headers = map[string]string{"X-Custom.Header!": "v"}
	if err := special.Validate(); err != nil {
		t.Errorf("Validate() special-char header error = %v, want nil", err)
	}

	bad := validOptions()
	bad.Headers = map[string]string{"a b": "v"}
	if err := bad.Validate(); err == nil {
		t.Fatal("Validate() = nil, want invalid header key error")
	}
}
