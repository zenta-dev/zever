package examples_test

import (
	"testing"
	"time"

	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/shared/codec"
)

// BenchmarkTypedCacheRoundTrip measures the typed cache helper over the
// in-memory backend: JSON encode, store, load, and decode.
func BenchmarkTypedCacheRoundTrip(b *testing.B) {
	backend, err := cachememory.New(cache.Options{})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	b.Cleanup(func() { _ = backend.Close(b.Context()) })

	typed := cache.NewTyped[string, spaceCard](backend, codec.JSONCodec[spaceCard]{})
	ctx := b.Context()
	card := spaceCard{Title: "Loft", PriceCents: 12000}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := typed.Set(ctx, "space:card:42", card, 5*time.Minute); err != nil {
			b.Fatalf("Set: %v", err)
		}
		if _, err := typed.Get(ctx, "space:card:42"); err != nil {
			b.Fatalf("Get: %v", err)
		}
	}
}
