package local

import (
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// TestEdgeEncryptDecrypt_emptyPlaintext covers the zero-length plaintext
// boundary: AES-GCM must round-trip an empty message without error.
func TestEdgeEncryptDecrypt_emptyPlaintext(t *testing.T) {
	t.Parallel()

	c := mustNew(t, crypto.Options{Key: genAESKey(t)})
	ctx := t.Context()

	ct, err := c.Encrypt(ctx, nil)
	if err != nil {
		t.Fatalf("Encrypt(nil) = %v, want nil", err)
	}

	got, err := c.Decrypt(ctx, ct)
	if err != nil {
		t.Fatalf("Decrypt = %v, want nil", err)
	}

	if len(got) != 0 {
		t.Fatalf("Decrypt = %q, want empty", got)
	}
}
