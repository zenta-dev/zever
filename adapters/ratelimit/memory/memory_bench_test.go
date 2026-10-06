package memory_test

import (
	"fmt"
	"testing"

	"github.com/zenta-dev/zever/adapters/ratelimit/memory"
	"github.com/zenta-dev/zever/core/ratelimit"
)

func benchKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%d", i)
	}
	return keys
}

// BenchmarkAllow measures the token-bucket decision on a single hot key.
func BenchmarkAllow(b *testing.B) {
	l, err := memory.New(ratelimit.Options{Rate: 1e9, Burst: 1000})
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}

	b.Cleanup(func() { _ = l.Close() })

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := l.Allow(ctx, "hot", 1); err != nil {
			b.Fatalf("Allow failed: %v", err)
		}
	}
}

func BenchmarkAllowParallel(b *testing.B) {
	l, err := memory.New(ratelimit.Options{Rate: 1000, Burst: 100})
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer func() { _ = l.Close() }()

	keys := benchKeys(256)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _ = l.Allow(ctx, keys[i&255], 1)
			i++
		}
	})
}

func BenchmarkAllowParallelHotKey(b *testing.B) {
	l, err := memory.New(ratelimit.Options{Rate: 1000, Burst: 100})
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}
	defer func() { _ = l.Close() }()

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = l.Allow(ctx, "hot", 1)
		}
	})
}
