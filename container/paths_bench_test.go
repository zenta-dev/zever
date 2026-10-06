package container

import (
	"testing"

	"github.com/zenta-dev/zever/config"
)

// BenchmarkContainer_ResolveEventbus measures resolving the eventbus alias
// from a cold container plus the empty shutdown that follows.
func BenchmarkContainer_ResolveEventbus(b *testing.B) {
	registerFakeAdapters()

	b.ReportAllocs()
	for b.Loop() {
		c := New(config.Default())
		if _, err := c.Eventbus(); err != nil {
			b.Fatalf("Eventbus: %v", err)
		}
		if err := c.Close(b.Context()); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
}

// BenchmarkContainer_ResolveRatelimit measures resolving the ratelimit alias
// from a cold container plus the empty shutdown that follows.
func BenchmarkContainer_ResolveRatelimit(b *testing.B) {
	registerFakeAdapters()

	b.ReportAllocs()
	for b.Loop() {
		c := New(config.Default())
		if _, err := c.Ratelimit(); err != nil {
			b.Fatalf("Ratelimit: %v", err)
		}
		if err := c.Close(b.Context()); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
}
