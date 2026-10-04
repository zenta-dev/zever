package password_test

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

var benchSeq atomic.Int64

func benchFreshAdapter() password.Adapter {
	return password.Adapter(fmt.Sprintf("bench-%d", 1000+benchSeq.Add(1)))
}

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := password.Register(benchFreshAdapter(), stubFactory); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func benchOptions() password.Options {
	return password.Options{
		Time:    password.DefaultTime,
		Memory:  password.DefaultMemory,
		Threads: password.DefaultThreads,
		SaltLen: password.DefaultSaltLen,
		KeyLen:  password.DefaultKeyLen,
	}
}

func BenchmarkOpen(b *testing.B) {
	a := benchFreshAdapter()
	if err := password.Register(a, stubFactory); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	opts := benchOptions()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := password.Open(a, opts); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := benchFreshAdapter()
	if err := password.Register(a, stubFactory); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	opts := benchOptions()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := password.Open(a, opts); err != nil {
				b.Fatalf("Open err = %v", err)
			}
		}
	})
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = password.ParseAdapter("argon2id")
	}
}

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
	for i := 0; i < b.N; i++ {
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
	for i := 0; i < b.N; i++ {
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
	for i := 0; i < b.N; i++ {
		if _, err := h.Verify(ctx, "hash:secret", "secret"); err != nil {
			b.Fatalf("Verify err = %v", err)
		}
	}
}
