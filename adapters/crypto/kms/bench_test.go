package kms

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
)

// benchOptions returns options enabling the deterministic stub client with
// both a MAC key and a signing key configured. The signing key is derived
// from a fixed seed so it is a valid, reproducible Ed25519 private key.
func benchOptions() Options {
	macKey := make([]byte, 32)
	signKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))

	return Options{
		KeyID:   "bench-key",
		DevStub: true,
		Key:     base64.StdEncoding.EncodeToString(macKey),
		SignKey: base64.StdEncoding.EncodeToString(signKey),
	}
}

// BenchmarkEncryptDecrypt measures envelope encryption plus decryption through
// the in-process stub KMS client.
func BenchmarkEncryptDecrypt(b *testing.B) {
	c := mustNew(b, benchOptions())
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
	c := mustNew(b, benchOptions())
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
	c := mustNew(b, benchOptions())
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
