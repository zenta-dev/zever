package noop

import (
	"testing"

	"github.com/zenta-dev/zever/core/observability"
)

func benchAttrs() []observability.Attr {
	return []observability.Attr{
		observability.String("k", "v"),
		observability.Int("n", 1),
		observability.Bool("b", true),
	}
}

// BenchmarkStartSpan measures the no-op span lifecycle hot path.
func BenchmarkStartSpan(b *testing.B) {
	tr := New().Tracer("bench")
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ctx, span := tr.Start(b.Context(), "op")
		span.SetAttributes(attrs...)
		span.RecordError(nil)
		span.End()
		_ = ctx
	}
}

// BenchmarkStartSpanParallel measures concurrent no-op span lifecycles.
func BenchmarkStartSpanParallel(b *testing.B) {
	tr := New().Tracer("bench")
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, span := tr.Start(b.Context(), "op")
			span.SetAttributes(attrs...)
			span.End()
		}
	})
}

// BenchmarkMetricsCounter measures the no-op counter hot path.
func BenchmarkMetricsCounter(b *testing.B) {
	m := New().Meter("bench")
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := m.Counter(b.Context(), "c", 1, attrs...); err != nil {
			b.Fatalf("Counter() = %v, want nil", err)
		}
	}
}

// BenchmarkNew measures constructing the no-op provider.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if New() == nil {
			b.Fatal("New() = nil")
		}
	}
}

// BenchmarkTracer measures acquiring a no-op tracer.
func BenchmarkTracer(b *testing.B) {
	p := New()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if p.Tracer("bench") == nil {
			b.Fatal("Tracer() = nil")
		}
	}
}

// BenchmarkMeter measures acquiring a no-op metrics handle.
func BenchmarkMeter(b *testing.B) {
	p := New()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if p.Meter("bench") == nil {
			b.Fatal("Meter() = nil")
		}
	}
}

// BenchmarkShutdown measures the no-op shutdown.
func BenchmarkShutdown(b *testing.B) {
	p := New()
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := p.Shutdown(ctx); err != nil {
			b.Fatal("Shutdown() =", err)
		}
	}
}

// BenchmarkGauge measures the no-op gauge hot path.
func BenchmarkGauge(b *testing.B) {
	m := New().Meter("bench")
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := m.Gauge(b.Context(), "g", 1, attrs...); err != nil {
			b.Fatalf("Gauge() = %v, want nil", err)
		}
	}
}

// BenchmarkHistogram measures the no-op histogram hot path.
func BenchmarkHistogram(b *testing.B) {
	m := New().Meter("bench")
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := m.Histogram(b.Context(), "h", 1, attrs...); err != nil {
			b.Fatalf("Histogram() = %v, want nil", err)
		}
	}
}

// BenchmarkSpanEnd measures the bare span close.
func BenchmarkSpanEnd(b *testing.B) {
	tr := New().Tracer("bench")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, span := tr.Start(b.Context(), "op")
		span.End()
	}
}
