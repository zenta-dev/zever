package otlp

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/zenta-dev/zever/core/observability"
)

func benchAttrs() []observability.Attr {
	return []observability.Attr{
		observability.String("k", "v"),
		observability.Int("n", 1),
		observability.Bool("b", true),
	}
}

func benchTracer(b *testing.B) *tracer {
	b.Helper()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	b.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return &tracer{tp: tp, tracer: tp.Tracer("bench")}
}

func benchMetrics(b *testing.B) *metrics {
	b.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	b.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return &metrics{
		mp:    mp,
		scope: "bench",
		cache: &instrumentCache{
			kinds:      make(map[string]instrumentKind),
			counters:   make(map[string]metric.Float64Counter),
			gauges:     make(map[string]metric.Float64Gauge),
			histograms: make(map[string]metric.Float64Histogram),
		},
	}
}

// BenchmarkSpanStartEnd measures the span lifecycle hot path (no exporter).
func BenchmarkSpanStartEnd(b *testing.B) {
	tr := benchTracer(b)
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, span := tr.Start(b.Context(), "op")
		span.SetAttributes(attrs...)
		span.End()
	}
}

// BenchmarkSpanStartEndParallel measures concurrent span lifecycles.
func BenchmarkSpanStartEndParallel(b *testing.B) {
	tr := benchTracer(b)
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

// BenchmarkNormalizeAttrs measures attribute normalization (cap/truncate/redact).
func BenchmarkNormalizeAttrs(b *testing.B) {
	attrs := []observability.Attr{
		observability.String("k", "value"),
		observability.String("password", "hunter2"),
		observability.Int("n", 1),
		observability.Bool("b", true),
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := normalizeAttrs(attrs, 0); len(got) != len(attrs) {
			b.Fatalf("normalizeAttrs() len = %d, want %d", len(got), len(attrs))
		}
	}
}

// BenchmarkMetricsCounter measures the counter instrument cache-hit and Add path.
func BenchmarkMetricsCounter(b *testing.B) {
	m := benchMetrics(b)
	attrs := benchAttrs()
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := m.Counter(ctx, "c", 1, attrs...); err != nil {
			b.Fatalf("Counter() = %v, want nil", err)
		}
	}
}
