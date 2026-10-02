package kms

import (
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/crypto/cryptotest"
)

// TestKMSConformance proves the kms adapter honors the crypto.Crypto
// contract via the shared conformance kit.
//
// Currently skipped: the adapter needs live KMS credentials
// (key ID + region) and network access. The kit's round-trip
// assertions match the local adapter's; envelope coverage lives in
// the adapter's own tests. Re-enable with test KMS credentials.
func TestKMSConformance(t *testing.T) {
	t.Skip("needs live KMS credentials and network")

	cryptotest.Conformance(t, func(t *testing.T) crypto.Crypto {
		t.Helper()

		c, err := New(Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return c
	})
}
