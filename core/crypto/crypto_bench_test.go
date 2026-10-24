package crypto_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// BenchmarkOptionsValidate measures Options validation for a crypto backend.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := validOpts()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate() err = %v", err)
		}
	}
}

// BenchmarkIsDevCryptoKey measures the dev-key detection hot path.
func BenchmarkIsDevCryptoKey(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !crypto.IsDevCryptoKey(crypto.DevCryptoKey) {
			b.Fatal("IsDevCryptoKey(DevCryptoKey) = false, want true")
		}
	}
}
