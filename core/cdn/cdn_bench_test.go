package cdn

import (
	"context"
	"testing"
)

// BenchmarkPurge_Noop measures purge of a single URL through a no-op backend.
func BenchmarkPurge_Noop(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := c.Purge(ctx, PurgeRequest{URLs: []string{"https://example.com/x"}}); err != nil {
			b.Fatalf("Purge: %v", err)
		}
	}
}

// BenchmarkPurge_Tags_Noop measures purge by cache tags.
func BenchmarkPurge_Tags_Noop(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := c.Purge(ctx, PurgeRequest{Tags: []string{"tag1", "tag2"}}); err != nil {
			b.Fatalf("Purge: %v", err)
		}
	}
}

// BenchmarkPurge_All_Noop measures the purge-all path.
func BenchmarkPurge_All_Noop(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := c.Purge(ctx, PurgeRequest{All: true}); err != nil {
			b.Fatalf("Purge: %v", err)
		}
	}
}

// BenchmarkClose_Noop measures the no-op close path.
func BenchmarkClose_Noop(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := c.Close(ctx); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
}

// noopAdapter discards purges and reports a fixed name for benchmarks.
type noopAdapter struct{}

func (n *noopAdapter) Purge(_ context.Context, _ PurgeRequest) error { return nil }
func (n *noopAdapter) Close(_ context.Context) error                 { return nil }
func (n *noopAdapter) Name() string                                  { return "bench-noop" }
