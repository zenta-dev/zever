package local

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// TestClose_ZeroesSecretsAndFailsOps proves Close wipes key material and
// fails every later operation instead of running keyless. The wire format
// is untouched: ciphertext sealed before Close keeps its nonce||ciphertext
// shape (it just can no longer be opened).
func TestClose_ZeroesSecretsAndFailsOps(t *testing.T) {
	t.Parallel()
	c := mustNew(t, crypto.Options{Key: genAESKey(t), SignKey: genSignKey(t)})
	ctx := t.Context()

	sealed, err := c.Encrypt(ctx, []byte("hello"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if _, err := c.Decrypt(ctx, sealed); err != nil {
		t.Fatalf("Decrypt() before Close error = %v, want nil", err)
	}

	closer, ok := c.(interface{ Close() error })
	if !ok {
		t.Fatal("crypto does not implement Close() error")
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("Close() second call error = %v, want nil", err)
	}

	if _, err := c.Encrypt(ctx, []byte("x")); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("Encrypt() after Close error = %v, want ErrKeyNotFound", err)
	}
	if _, err := c.Decrypt(ctx, sealed); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("Decrypt() after Close error = %v, want ErrKeyNotFound", err)
	}
	if _, err := c.Sign(ctx, []byte("m")); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("Sign() after Close error = %v, want ErrKeyNotFound", err)
	}
	if _, err := c.Verify(ctx, []byte("m"), []byte("s")); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("Verify() after Close error = %v, want ErrKeyNotFound", err)
	}
	if _, err := c.Mac(ctx, []byte("m")); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("Mac() after Close error = %v, want ErrKeyNotFound", err)
	}
	if _, err := c.VerifyMac(ctx, []byte("m"), []byte("x")); !errors.Is(err, crypto.ErrKeyNotFound) {
		t.Fatalf("VerifyMac() after Close error = %v, want ErrKeyNotFound", err)
	}
}
