package crypto_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

func BenchmarkOpen(b *testing.B) {
	a := freshCryptoAdapter()
	opts := validOpts()
	if err := crypto.Register(a, func(crypto.Options) (crypto.Crypto, error) { return &mockCrypto{}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := crypto.Open(a, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := freshCryptoAdapter()
	opts := validOpts()
	if err := crypto.Register(a, func(crypto.Options) (crypto.Crypto, error) { return &mockCrypto{}, nil }); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := crypto.Open(a, opts); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkOptionsValidate(b *testing.B) {
	opts := validOpts()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := opts.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncrypt(b *testing.B) {
	m := &mockCrypto{}
	plaintext := []byte("the quick brown fox jumps over the lazy dog")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := m.Encrypt(b.Context(), plaintext); err != nil {
			b.Fatal(err)
		}
	}
}
