package crypto_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

func validOpts() crypto.Options {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return crypto.Options{Key: base64.StdEncoding.EncodeToString(k)}
}

type mockCrypto struct{}

func (m *mockCrypto) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	out := make([]byte, len(plaintext))
	for i, b := range plaintext {
		out[i] = b + 1
	}
	return out, nil
}

func (m *mockCrypto) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	out := make([]byte, len(ciphertext))
	for i, b := range ciphertext {
		out[i] = b - 1
	}
	return out, nil
}

func (m *mockCrypto) Sign(_ context.Context, message []byte) ([]byte, error) {
	cp := make([]byte, len(message))
	copy(cp, message)
	return cp, nil
}

func (m *mockCrypto) Verify(_ context.Context, message, signature []byte) (bool, error) {
	if len(message) != len(signature) {
		return false, nil
	}
	for i := range message {
		if message[i] != signature[i] {
			return false, nil
		}
	}
	return true, nil
}

func (m *mockCrypto) Mac(_ context.Context, message []byte) ([]byte, error) {
	h := sha256.Sum256(message)
	out := make([]byte, len(h))
	copy(out, h[:])
	return out, nil
}

func (m *mockCrypto) VerifyMac(_ context.Context, message, mac []byte) (bool, error) {
	h := sha256.Sum256(message)
	if len(mac) != len(h) {
		return false, nil
	}
	for i := range h {
		if mac[i] != h[i] {
			return false, nil
		}
	}
	return true, nil
}

func TestCrypto_mock_increment_and_mac(t *testing.T) {
	t.Parallel()
	m := &mockCrypto{}
	ctx := t.Context()
	in := []byte{0, 255, 10}
	enc, _ := m.Encrypt(ctx, in)
	if enc[0] != 1 || enc[1] != 0 || enc[2] != 11 {
		t.Fatalf("Encrypt increment failed: %v", enc)
	}
	dec, _ := m.Decrypt(ctx, enc)
	for i := range in {
		if dec[i] != in[i] {
			t.Fatalf("Decrypt mismatch at %d", i)
		}
	}
	mac, _ := m.Mac(ctx, []byte("abc"))
	if len(mac) != sha256.Size {
		t.Fatalf("Mac len = %d, want %d", len(mac), sha256.Size)
	}
	ok, _ := m.VerifyMac(ctx, []byte("abc"), mac)
	if !ok {
		t.Fatal("VerifyMac expected true")
	}
	ok, _ = m.VerifyMac(ctx, []byte("abc"), []byte("bad"))
	if ok {
		t.Fatal("VerifyMac expected false for bad mac")
	}
}
