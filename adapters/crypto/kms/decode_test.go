package kms

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// TestNewWithClient_MalformedKeys_FailClosed pins the P2-11d fix:
// base64 decode failures for Key/SignKey must surface as errors wrapping
// crypto.ErrInvalidKey, never as silently zero-valued keys.
func TestNewWithClient_MalformedKeys_FailClosed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts Options
	}{
		{"key bad base64", Options{KeyID: "k1", Key: "!!!not-base64!!!"}},
		{"key wrong length", Options{KeyID: "k1", Key: "aGk="}},
		{"sign key bad base64", Options{KeyID: "k1", SignKey: "!!!not-base64!!!"}},
		{"sign key wrong length", Options{KeyID: "k1", SignKey: "aGk="}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewWithClient(tc.opts, StubClient()); !errors.Is(err, crypto.ErrInvalidKey) {
				t.Fatalf("NewWithClient err = %v, want ErrInvalidKey", err)
			}
		})
	}
}

// TestSign_EmptyPrivateKey_FailsClosed pins the panic guard: a driver
// carrying an empty signing key must fail with ErrKeyNotFound, never panic
// inside ed25519.Sign.
func TestSign_EmptyPrivateKey_FailsClosed(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	for name, d := range map[string]*driver{
		"nil key":   {},
		"empty key": {signPriv: ed25519.PrivateKey{}},
	} {
		if _, err := d.Sign(ctx, []byte("m")); !errors.Is(err, crypto.ErrKeyNotFound) {
			t.Errorf("%s: Sign err = %v, want ErrKeyNotFound", name, err)
		}
		if _, err := d.Verify(ctx, []byte("m"), []byte("s")); !errors.Is(err, crypto.ErrKeyNotFound) {
			t.Errorf("%s: Verify err = %v, want ErrKeyNotFound", name, err)
		}
	}
}
