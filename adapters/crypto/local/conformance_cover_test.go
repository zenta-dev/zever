package local

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/crypto/cryptotest"
)

// testKeys returns deterministic test keys (never production): a
// 32-byte AES key and a 64-byte ed25519 private key built with
// NewKeyFromSeed so the stored public half matches the seed
// (PrivateKey.Public returns the stored half without recomputing).
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

// TestLocalConformance proves the local adapter honors the
// crypto.Crypto contract via the shared conformance kit. Keys are
// deterministic test keys (never production); each subtest gets a
// fresh instance.
func TestLocalConformance(t *testing.T) {
	cryptotest.Conformance(t, func(t *testing.T) crypto.Crypto {
		t.Helper()

		key, signKey := testKeys()

		c, err := New(crypto.Options{Key: key, SignKey: signKey})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return c
	})
}
