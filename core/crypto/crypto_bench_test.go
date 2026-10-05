package crypto_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

// benchAdapter registers a stub crypto backend once and returns its adapter
// so Open can be measured without paying the registration cost.
func benchAdapter(b *testing.B) crypto.Adapter {
	b.Helper()

	a := freshCryptoAdapter()
	if err := crypto.Register(a, func(crypto.Options) (crypto.Crypto, error) { return &mockCrypto{}, nil }); err != nil {
		b.Fatalf("Register(%v) err = %v", a, err)
	}

	return a
}

// BenchmarkRegister measures the registry registration hot path.
func BenchmarkRegister(b *testing.B) {
	factory := func(crypto.Options) (crypto.Crypto, error) { return &mockCrypto{}, nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		a := freshCryptoAdapter()
		if err := crypto.Register(a, factory); err != nil {
			b.Fatalf("Register(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpen measures the registry lookup plus factory invocation hot path.
func BenchmarkOpen(b *testing.B) {
	a := benchAdapter(b)
	opts := validOpts()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := crypto.Open(a, opts); err != nil {
			b.Fatalf("Open(%v) err = %v", a, err)
		}
	}
}

// BenchmarkOpenParallel measures concurrent Open calls on one adapter.
func BenchmarkOpenParallel(b *testing.B) {
	a := benchAdapter(b)
	opts := validOpts()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := crypto.Open(a, opts); err != nil {
				b.Errorf("Open(%v) err = %v", a, err)
				return
			}
		}
	})
}

// BenchmarkOptionsValidate measures options validation with a valid key.
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
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := crypto.ParseAdapter("local"); err != nil {
			b.Fatalf("ParseAdapter() err = %v", err)
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
