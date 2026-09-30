package kms

import (
	"bytes"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

func mustNew(t *testing.T, opts Options) crypto.Crypto {
	t.Helper()
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	return c
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	t.Parallel()
	c := mustNew(t, Options{KeyID: "test-key-a"})
	ctx := t.Context()
	plaintext := []byte("hello kms envelope")
	enc, err := c.Encrypt(ctx, plaintext)
	if err != nil {
		t.Fatalf("Encrypt err = %v", err)
	}
	dec, err := c.Decrypt(ctx, enc)
	if err != nil {
		t.Fatalf("Decrypt err = %v", err)
	}
	if !bytes.Equal(dec, plaintext) {
		t.Fatalf("roundtrip mismatch: got %q want %q", dec, plaintext)
	}
	if bytes.Equal(enc, plaintext) {
		t.Fatal("ciphertext should not equal plaintext")
	}
}

func TestRotation_DecryptWithKeyIDs(t *testing.T) {
	t.Parallel()
	c1 := mustNew(t, Options{KeyID: "key-a"})
	ctx := t.Context()
	enc, err := c1.Encrypt(ctx, []byte("rotate me"))
	if err != nil {
		t.Fatalf("Encrypt err = %v", err)
	}
	// New primary B, but allow A for rotation.
	c2 := mustNew(t, Options{KeyID: "key-b", KeyIDs: []string{"key-a"}})
	dec, err := c2.Decrypt(ctx, enc)
	if err != nil {
		t.Fatalf("Decrypt after rotation err = %v", err)
	}
	if string(dec) != "rotate me" {
		t.Fatalf("got %q, want %q", dec, "rotate me")
	}
}

func TestDecrypt_WrongKey_Fails(t *testing.T) {
	t.Parallel()
	c1 := mustNew(t, Options{KeyID: "key-a"})
	c2 := mustNew(t, Options{KeyID: "key-b"})
	ctx := t.Context()
	enc, err := c1.Encrypt(ctx, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt err = %v", err)
	}
	if _, err := c2.Decrypt(ctx, enc); !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("got err %v, want ErrIntegrity", err)
	}
}

func TestDecrypt_Tampered_Fails(t *testing.T) {
	t.Parallel()
	c := mustNew(t, Options{KeyID: "key-a"})
	ctx := t.Context()
	enc, err := c.Encrypt(ctx, []byte("tamper me"))
	if err != nil {
		t.Fatalf("Encrypt err = %v", err)
	}
	enc[len(enc)-1] ^= 0xff
	if _, err := c.Decrypt(ctx, enc); !errors.Is(err, crypto.ErrIntegrity) {
		t.Fatalf("got err %v, want ErrIntegrity", err)
	}
}

func TestNew_EmptyKeyID_FailsClosed(t *testing.T) {
	t.Parallel()
	if _, err := New(Options{}); err == nil {
		t.Fatal("expected error for empty KeyID, got nil")
	}
	if _, err := New(Options{KeyID: "  "}); err == nil {
		t.Fatal("expected error for blank KeyID, got nil")
	}
	if _, err := NewWithClient(Options{}, StubClient()); err == nil {
		t.Fatal("expected error for empty KeyID with client, got nil")
	}
}
