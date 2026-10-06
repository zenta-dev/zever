package stdout_test

import (
	"errors"
	"io"
	"testing"

	"github.com/zenta-dev/zever/adapters/observability/stdout"
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

func benchProvider(b *testing.B, mut func(*observability.Options)) observability.Provider {
	b.Helper()
	opts := validOptions()
	if mut != nil {
		mut(&opts)
	}
	p, err := stdout.NewWithWriter(opts, io.Discard)
	if err != nil {
		b.Fatalf("NewWithWriter() = %v", err)
	}
	return p
}

// BenchmarkSpanEnd measures the span lifecycle and JSON emit hot path against
// io.Discard.
func BenchmarkSpanEnd(b *testing.B) {
	p := benchProvider(b, nil)
	tr := p.Tracer("bench")
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, span := tr.Start(b.Context(), "op")
		span.SetAttributes(attrs...)
		span.End()
	}
}

// BenchmarkSpanEndParallel measures concurrent span emit through the shared
// mutex-guarded writer.
func BenchmarkSpanEndParallel(b *testing.B) {
	p := benchProvider(b, nil)
	tr := p.Tracer("bench")
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

// BenchmarkMetricsCounterVerbose measures the verbose metric emit path.
func BenchmarkMetricsCounterVerbose(b *testing.B) {
	p := benchProvider(b, func(o *observability.Options) { o.Verbose = true })
	m := p.Meter("bench")
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := m.Counter(b.Context(), "c", 1, attrs...); err != nil {
			b.Fatalf("Counter() = %v, want nil", err)
		}
	}
}

// BenchmarkNewWithWriter measures constructing the provider over io.Discard.
func BenchmarkNewWithWriter(b *testing.B) {
	opts := validOptions()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		p, err := stdout.NewWithWriter(opts, io.Discard)
		if err != nil {
			b.Fatalf("NewWithWriter() = %v", err)
		}
		if p == nil {
			b.Fatal("NewWithWriter() = nil")
		}
	}
}

// BenchmarkMetricsCounterNonVerbose measures the quiet no-op metric path.
func BenchmarkMetricsCounterNonVerbose(b *testing.B) {
	p := benchProvider(b, nil)
	m := p.Meter("bench")
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := m.Counter(b.Context(), "c", 1, attrs...); err != nil {
			b.Fatalf("Counter() = %v, want nil", err)
		}
	}
}

// BenchmarkSpanRecordError measures recording an error on an open span.
func BenchmarkSpanRecordError(b *testing.B) {
	p := benchProvider(b, nil)
	tr := p.Tracer("bench")
	_, span := tr.Start(b.Context(), "op")
	b.Cleanup(func() { span.End() })
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		span.RecordError(errBench)
	}
}

// BenchmarkSpanEndWithError measures the span close path with an error set.
func BenchmarkSpanEndWithError(b *testing.B) {
	p := benchProvider(b, nil)
	tr := p.Tracer("bench")
	attrs := benchAttrs()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, span := tr.Start(b.Context(), "op")
		span.SetAttributes(attrs...)
		span.RecordError(errBench)
		span.End()
	}
}
