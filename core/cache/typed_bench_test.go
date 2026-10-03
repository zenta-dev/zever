package cache_test

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/core/cache"
)

func BenchmarkTypedKeyString(b *testing.B) {
	backend, err := memory.New(cache.Options{})
	if err != nil {
		b.Fatalf("memory.New() error = %v", err)
	}

	defer backend.Close(b.Context())

	tc := cache.NewTyped[string, string](backend, prefixCodec{})

	if setErr := tc.Set(b.Context(), "k", "v", time.Minute); setErr != nil {
		b.Fatalf("Set() error = %v", setErr)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := tc.Set(b.Context(), "k", "v", time.Minute); err != nil {
			b.Fatalf("Set() error = %v", err)
		}

		if _, err := tc.Get(b.Context(), "k"); err != nil {
			b.Fatalf("Get() error = %v", err)
		}
	}
}

func BenchmarkTypedKeyInt(b *testing.B) {
	backend, err := memory.New(cache.Options{})
	if err != nil {
		b.Fatalf("memory.New() error = %v", err)
	}

	defer backend.Close(b.Context())

	tc := cache.NewTyped[int64, string](backend, prefixCodec{})

	if setErr := tc.Set(b.Context(), 42, "v", time.Minute); setErr != nil {
		b.Fatalf("Set() error = %v", setErr)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := tc.Set(b.Context(), 42, "v", time.Minute); err != nil {
			b.Fatalf("Set() error = %v", err)
		}

		if _, err := tc.Get(b.Context(), int64(42)); err != nil {
			b.Fatalf("Get() error = %v", err)
		}
	}
}
