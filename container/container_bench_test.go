package container

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
)

// benchProviders wires the shutdown-critical subset (db, cache, queue,
// scheduler, job) to zero-value fakes for benchmarks.
func benchProviders() []Provider {
	return []Provider{
		Provide(db.DefaultRef(), newFakeDB),
		Provide(cache.DefaultRef(), newFakeCache),
		Provide(queue.DefaultRef(), newFakeQueue),
		Provide(scheduler.DefaultRef(), newFakeScheduler),
	}
}

// BenchmarkContainer_ResolveSubset measures resolving the shutdown-critical
// subset (db, cache, queue, scheduler, job) from a cold container.
func BenchmarkContainer_ResolveSubset(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		c := New(nil, benchProviders()...)
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
	c := New(nil, benchProviders()...)
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
	for b.Loop() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = c.Close(ctx)
		cancel()
	}
}

// BenchmarkContainer_DBAlreadyResolved exercises the accessor fast path:
// Container.DB() after warmup is a provider-cache hit only.
func BenchmarkContainer_DBAlreadyResolved(b *testing.B) {
	c := New(nil, benchProviders()...)
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
