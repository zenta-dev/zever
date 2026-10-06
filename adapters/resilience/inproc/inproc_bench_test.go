package inproc_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/adapters/resilience/inproc"
	"github.com/zenta-dev/zever/core/resilience"
)

func mustBenchManager(b *testing.B, opts resilience.Options) resilience.Manager {
	b.Helper()

	m, err := inproc.New(opts)
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	b.Cleanup(func() { _ = m.Close() })

	return m
}

func mustBenchGuard(b *testing.B, m resilience.Manager) resilience.Guard {
	b.Helper()

	g, err := m.Guard("dep")
	if err != nil {
		b.Fatalf("Guard(%q) error = %v", "dep", err)
	}

	return g
}

func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		m, err := inproc.New(resilience.Options{})
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		if err := m.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

func BenchmarkManagerGuardCached(b *testing.B) {
	m := mustBenchManager(b, resilience.Options{})
	mustBenchGuard(b, m)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := m.Guard("dep"); err != nil {
			b.Fatalf("Guard() error = %v", err)
		}
	}
}

func BenchmarkGuardExecuteNoop(b *testing.B) {
	m := mustBenchManager(b, resilience.Options{})
	g := mustBenchGuard(b, m)
	ctx := b.Context()
	fn := func(context.Context) error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := g.Execute(ctx, fn); err != nil {
			b.Fatalf("Execute() error = %v", err)
		}
	}
}

func BenchmarkGuardExecuteBreaker(b *testing.B) {
	m := mustBenchManager(b, resilience.Options{
		Breaker: resilience.BreakerOptions{Enabled: true},
	})
	g := mustBenchGuard(b, m)
	ctx := b.Context()
	fn := func(context.Context) error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := g.Execute(ctx, fn); err != nil {
			b.Fatalf("Execute() error = %v", err)
		}
	}
}

func BenchmarkGuardExecuteBulkhead(b *testing.B) {
	m := mustBenchManager(b, resilience.Options{
		Bulkhead: resilience.BulkheadOptions{MaxConcurrent: 64, MaxQueue: 64},
	})
	g := mustBenchGuard(b, m)
	ctx := b.Context()
	fn := func(context.Context) error { return nil }

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := g.Execute(ctx, fn); err != nil {
			b.Fatalf("Execute() error = %v", err)
		}
	}
}
