package cryptotest_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"

	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/crypto/cryptotest"
)

// testKeys returns deterministic test keys (never production); see the
// adapter wiring for why NewKeyFromSeed is required.
func testKeys() (key, signKey string) {
	aesKey := make([]byte, 32)
	for i := range aesKey {
		aesKey[i] = byte(i + 1)
	}

	var seed [32]byte
	for i := range seed {
		seed[i] = byte(0xff - i)
	}

	return base64.StdEncoding.EncodeToString(aesKey),
		base64.StdEncoding.EncodeToString(ed25519.NewKeyFromSeed(seed[:]))
}

// TestConformanceLocal proves the kit passes against the local adapter.
// Keys are deterministic test keys (never production).
func TestConformanceLocal(t *testing.T) {
	t.Parallel()

	cryptotest.Conformance(t, func(t *testing.T) crypto.Crypto {
		t.Helper()

		key, signKey := testKeys()

		c, err := cryptolocal.New(crypto.Options{Key: key, SignKey: signKey})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return c
	})
}
