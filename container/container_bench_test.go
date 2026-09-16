package container

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
)

func testConfigMemoryDB() *config.Config {
	cfg := config.Default()
	cfg.DB.Options.Path = ":memory:"
	return cfg
}

// BenchmarkContainer_ResolveSubset measures resolving the shutdown-critical
// subset (db, cache, queue, scheduler, job) from a cold container.
func BenchmarkContainer_ResolveSubset(b *testing.B) {
	registerTestAdapters()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c := New(testConfigMemoryDB())
		if _, err := c.DB(); err != nil {
			b.Fatalf("DB: %v", err)
		}
		if _, err := c.Cache(); err != nil {
			b.Fatalf("Cache: %v", err)
		}
		if _, err := c.Queue(); err != nil {
			b.Fatalf("Queue: %v", err)
		}
		if _, err := c.Scheduler(); err != nil {
			b.Fatalf("Scheduler: %v", err)
		}
		if _, err := c.Job(); err != nil {
			b.Fatalf("Job: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = c.Close(ctx)
		cancel()
	}
}

// BenchmarkContainer_Close_ResolvedSubset measures Close over an already
// resolved subset: the steady-state shutdown cost.
func BenchmarkContainer_Close_ResolvedSubset(b *testing.B) {
	registerTestAdapters()
	c := New(testConfigMemoryDB())
	if _, err := c.DB(); err != nil {
		b.Fatalf("DB: %v", err)
	}
	if _, err := c.Cache(); err != nil {
		b.Fatalf("Cache: %v", err)
	}
	if _, err := c.Queue(); err != nil {
		b.Fatalf("Queue: %v", err)
	}
	if _, err := c.Scheduler(); err != nil {
		b.Fatalf("Scheduler: %v", err)
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = c.Close(ctx)
		cancel()
	}
}

// BenchmarkContainer_DBAlreadyResolved exercises the accessor fast path:
// Container.DB() after warmup is an atomic load only.
func BenchmarkContainer_DBAlreadyResolved(b *testing.B) {
	registerTestAdapters()
	c := New(testConfigMemoryDB())
	if _, err := c.DB(); err != nil {
		b.Fatalf("warmup DB: %v", err)
	}
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.Close(ctx)
	})
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := c.DB(); err != nil {
				b.Fatalf("DB: %v", err)
			}
		}
	})
}
