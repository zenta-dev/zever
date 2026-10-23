package crypto_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// benchAdapter registers a stub crypto backend once and returns its adapter
// so Open can be measured without paying the registration cost.
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

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkIsDevCryptoKey(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if !crypto.IsDevCryptoKey(crypto.DevCryptoKey) {
			b.Fatal("IsDevCryptoKey(DevCryptoKey) = false, want true")
		}
	}
}
