package local

import (
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// benchOptions returns options with generated AES and signing keys.
func benchOptions(tb testing.TB) crypto.Options {
	tb.Helper()

	return crypto.Options{Key: genAESKey(tb), SignKey: genSignKey(tb)}
}

// BenchmarkEncryptDecrypt measures AES-256-GCM seal plus open.
func BenchmarkEncryptDecrypt(b *testing.B) {
	c := mustNew(b, benchOptions(b))
	ctx := b.Context()
	plaintext := []byte("benchmark plaintext payload")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ct, err := c.Encrypt(ctx, plaintext)
		if err != nil {
			b.Fatal(err)
		}

		if _, err := c.Decrypt(ctx, ct); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSignVerify measures Ed25519 sign plus verify.
func BenchmarkSignVerify(b *testing.B) {
	c := mustNew(b, benchOptions(b))
	ctx := b.Context()
	msg := []byte("message to sign")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sig, err := c.Sign(ctx, msg)
		if err != nil {
			b.Fatal(err)
		}

		ok, err := c.Verify(ctx, msg, sig)
		if err != nil {
			b.Fatal(err)
		}

		if !ok {
			b.Fatal("signature did not verify")
		}
	}
}

// BenchmarkMacVerifyMac measures HMAC-SHA256 plus verification.
func BenchmarkMacVerifyMac(b *testing.B) {
	c := mustNew(b, benchOptions(b))
	ctx := b.Context()
	msg := []byte("message to authenticate")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		mac, err := c.Mac(ctx, msg)
		if err != nil {
			b.Fatal(err)
		}

		ok, err := c.VerifyMac(ctx, msg, mac)
		if err != nil {
			b.Fatal(err)
		}

		if !ok {
			b.Fatal("mac did not verify")
		}
	}
}
