package argon2

import (
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

// benchHasher uses the minimum accepted cost so benchmarks stay fast while
// still exercising the real argon2id derivation path.
func benchHasher(b *testing.B) password.Hasher {
	b.Helper()
	h, err := New(password.Options{Time: 1, Memory: 8 * 1024, Threads: 1, SaltLen: 8, KeyLen: 16})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	return h
}

// BenchmarkHash measures the salt-generate and key-derivation hot path.
func BenchmarkHash(b *testing.B) {
	h := benchHasher(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := h.Hash(ctx, "correct horse battery staple"); err != nil {
			b.Fatalf("Hash() = %v, want nil", err)
		}
	}
}

// BenchmarkVerify measures the decode and constant-time compare hot path.
func BenchmarkVerify(b *testing.B) {
	h := benchHasher(b)
	ctx := b.Context()
	hash, err := h.Hash(ctx, "correct horse battery staple")
	if err != nil {
		b.Fatalf("Hash() = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ok, err := h.Verify(ctx, hash, "correct horse battery staple")
		if err != nil {
			b.Fatalf("Verify() = %v", err)
		}
		if !ok {
			b.Fatal("Verify() = false, want true")
		}
	}
}

// BenchmarkNeedsRehash measures the decode-and-compare hot path.
func BenchmarkNeedsRehash(b *testing.B) {
	h := benchHasher(b)
	ctx := b.Context()
	hash, err := h.Hash(ctx, "secret")
	if err != nil {
		b.Fatalf("Hash() = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := h.NeedsRehash(ctx, hash); err != nil {
			b.Fatalf("NeedsRehash() = %v", err)
		}
	}
}

// BenchmarkHashParallel measures concurrent hashing (each call allocates an
// independent salt).
func BenchmarkHashParallel(b *testing.B) {
	h := benchHasher(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := h.Hash(ctx, "secret"); err != nil {
				b.Errorf("Hash() = %v", err)
				return
			}
		}
	})
}

// BenchmarkRegister measures the exported registry-wiring entrypoint.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		Register()
	}
}
