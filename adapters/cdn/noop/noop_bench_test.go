package noop

import (
	"testing"

	"github.com/zenta-dev/zever/core/cdn"
)

// BenchmarkNew measures the no-op CDN construction.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		c, err := New(cdn.Options{})
		if err != nil {
			b.Fatalf("New: %v", err)
		}
		_ = c
	}
}

// BenchmarkPurge_URLs measures the purge by URLs path.
func BenchmarkPurge_URLs(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := c.Purge(ctx, cdn.PurgeRequest{URLs: []string{"https://example.com/style.css", "https://example.com/script.js"}}); err != nil {
			b.Fatalf("Purge: %v", err)
		}
	}
}

// BenchmarkPurge_Tags measures the purge by tags path.
func BenchmarkPurge_Tags(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := c.Purge(ctx, cdn.PurgeRequest{Tags: []string{"tag1", "tag2", "tag3"}}); err != nil {
			b.Fatalf("Purge: %v", err)
		}
	}
}

// BenchmarkPurge_All measures the purge all path.
func BenchmarkPurge_All(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := c.Purge(ctx, cdn.PurgeRequest{All: true}); err != nil {
			b.Fatalf("Purge: %v", err)
		}
	}
}

// BenchmarkPurge_Empty measures the empty purge request (no-op).
func BenchmarkPurge_Empty(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := c.Purge(ctx, cdn.PurgeRequest{}); err != nil {
			b.Fatalf("Purge: %v", err)
		}
	}
}

// BenchmarkClose measures the no-op close path.
func BenchmarkClose(b *testing.B) {
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

// BenchmarkName measures the adapter name lookup.
func BenchmarkName(b *testing.B) {
	c := &noopAdapter{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if got := c.Name(); got != "noop" {
			b.Fatalf("Name: %s", got)
		}
	}
}

// BenchmarkPurge_Parallel measures concurrent purge operations.
func BenchmarkPurge_Parallel(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := c.Purge(ctx, cdn.PurgeRequest{URLs: []string{"https://example.com/x"}}); err != nil {
				b.Fatalf("Purge: %v", err)
			}
		}
	})
}

// BenchmarkPurge_SingleURL measures single URL purge.
func BenchmarkPurge_SingleURL(b *testing.B) {
	c := &noopAdapter{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := c.Purge(ctx, cdn.PurgeRequest{URLs: []string{"https://example.com/single.css"}}); err != nil {
			b.Fatalf("Purge: %v", err)
		}
	}
}
