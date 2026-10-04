package kms

import "testing"

// TestEdgeEncryptDecrypt_emptyPlaintext covers the zero-length plaintext
// boundary through the in-process stub client.
func TestEdgeEncryptDecrypt_emptyPlaintext(t *testing.T) {
	t.Parallel()

	c := mustNew(t, Options{KeyID: "test-key-a", DevStub: true})
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
