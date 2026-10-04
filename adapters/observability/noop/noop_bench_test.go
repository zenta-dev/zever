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
	for i := 0; i < b.N; i++ {
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
	for i := 0; i < b.N; i++ {
		if err := m.Counter(b.Context(), "c", 1, attrs...); err != nil {
			b.Fatalf("Counter() = %v, want nil", err)
		}
	}
}
