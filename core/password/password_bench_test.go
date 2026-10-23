package password_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

func BenchmarkOptionsValidate(b *testing.B) {
	o := password.Options{
		Time:    password.DefaultTime,
		Memory:  password.DefaultMemory,
		Threads: password.DefaultThreads,
		SaltLen: password.DefaultSaltLen,
		KeyLen:  password.DefaultKeyLen,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := o.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

func BenchmarkHasherHash(b *testing.B) {
	h := stubHasher{}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := h.Hash(ctx, "benchmark-password"); err != nil {
			b.Fatalf("Hash err = %v", err)
		}
	}
}

func BenchmarkHasherVerify(b *testing.B) {
	h := stubHasher{}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := h.Verify(ctx, "hash:secret", "secret"); err != nil {
			b.Fatalf("Verify err = %v", err)
		}
	}
}
