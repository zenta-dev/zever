package otlp

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/zenta-dev/zever/core/observability"
)

var errBench = errors.New("bench error")

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
	for b.Loop() {
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
	for b.Loop() {
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
	for b.Loop() {
		if err := m.Counter(ctx, "c", 1, attrs...); err != nil {
			b.Fatalf("Counter() = %v, want nil", err)
		}
	}
}

// BenchmarkMetricsGauge measures the gauge instrument cache-hit and Record path.
func BenchmarkMetricsGauge(b *testing.B) {
	m := benchMetrics(b)
	attrs := benchAttrs()
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := m.Gauge(ctx, "g", 1, attrs...); err != nil {
			b.Fatalf("Gauge() = %v, want nil", err)
		}
	}
}

// BenchmarkMetricsHistogram measures the histogram instrument cache-hit and Record path.
func BenchmarkMetricsHistogram(b *testing.B) {
	m := benchMetrics(b)
	attrs := benchAttrs()
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := m.Histogram(ctx, "h", 1, attrs...); err != nil {
			b.Fatalf("Histogram() = %v, want nil", err)
		}
	}
}

// BenchmarkSpanRecordError measures recording an error on a span.
func BenchmarkSpanRecordError(b *testing.B) {
	tr := benchTracer(b)
	_, span := tr.Start(b.Context(), "op")
	b.Cleanup(func() { span.End() })
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		span.RecordError(errBench)
	}
}

// BenchmarkIsSensitiveKey measures the secret-key substring scan.
func BenchmarkIsSensitiveKey(b *testing.B) {
	keys := []string{"user", "password", "api_key", "session", "plain_key"}
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		_ = isSensitiveKey(keys[i%len(keys)])
		i++
	}
}

// BenchmarkToAttribute measures converting a zever attribute onto an otel one.
func BenchmarkToAttribute(b *testing.B) {
	attrs := []observability.Attr{
		observability.String("s", "v"),
		observability.Int64("i", 1),
		observability.Float64("f", 1.5),
		observability.Bool("b", true),
	}
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		a := attrs[i%len(attrs)]
		_ = toAttribute(a.Key, a.Value)
		i++
	}
}

// BenchmarkKindMismatchErrorString measures formatting a kind mismatch error.
func BenchmarkKindMismatchErrorString(b *testing.B) {
	err := kindMismatch("requests", kindCounter, kindGauge)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = err.Error()
	}
}

// BenchmarkNew measures constructing the full provider against an insecure
// loopback endpoint. Providers are intentionally not shut down: export
// timeouts dominate the measurement and construction is the hot path.
func BenchmarkNew(b *testing.B) {
	opts := observability.Options{
		ServiceName: "bench",
		Endpoint:    "127.0.0.1:1",
		Insecure:    true,
		SampleRatio: 1,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := New(opts); err != nil {
			b.Fatalf("New() = %v", err)
		}
	}
}
