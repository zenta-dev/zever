package redis_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/adapters/resilience/redis"
	"github.com/zenta-dev/zever/core/resilience"
	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

// benchServer starts one shared miniredis instance per benchmark.
func benchServer(b *testing.B) *miniredis.Miniredis {
	b.Helper()

	s, err := miniredis.Run()
	if err != nil {
		b.Fatalf("miniredis Run() error = %v", err)
	}

	b.Cleanup(s.Close)

	return s
}

// benchOptions points template options at the benchmark server.
func benchOptions(s *miniredis.Miniredis, opts resilience.Options) redis.Options {
	opts.Redis.Options = redisopt.Options{
		ConnectOptions: redisopt.ConnectOptions{Addr: s.Addr()},
	}

	return redis.Options{Options: opts}
}

func mustBenchManager(b *testing.B, opts redis.Options) resilience.Manager {
	b.Helper()

	m, err := redis.New(opts)
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
		b.Fatalf("Guard() error = %v", err)
	}

	return g
}

func BenchmarkRedisNew(b *testing.B) {
	s := benchServer(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		m, err := redis.New(benchOptions(s, resilience.Options{}))
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		if err := m.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

func BenchmarkRedisGuardCached(b *testing.B) {
	m := mustBenchManager(b, benchOptions(benchServer(b), resilience.Options{}))
	mustBenchGuard(b, m)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := m.Guard("dep"); err != nil {
			b.Fatalf("Guard() error = %v", err)
		}
	}
}

func BenchmarkRedisExecuteNoop(b *testing.B) {
	m := mustBenchManager(b, benchOptions(benchServer(b), resilience.Options{}))
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

func BenchmarkRedisExecuteBreaker(b *testing.B) {
	m := mustBenchManager(b, benchOptions(benchServer(b), resilience.Options{
		Breaker: resilience.BreakerOptions{Enabled: true},
	}))
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

func BenchmarkRedisExecuteBulkhead(b *testing.B) {
	m := mustBenchManager(b, benchOptions(benchServer(b), resilience.Options{
		Bulkhead: resilience.BulkheadOptions{MaxConcurrent: 64, MaxQueue: 64},
	}))
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
