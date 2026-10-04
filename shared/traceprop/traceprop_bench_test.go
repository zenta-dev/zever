package traceprop

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func benchTraceID(b *testing.B) trace.TraceID {
	b.Helper()

	id, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		b.Fatalf("TraceIDFromHex: %v", err)
	}

	return id
}

func benchSpanID(b *testing.B) trace.SpanID {
	b.Helper()

	id, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		b.Fatalf("SpanIDFromHex: %v", err)
	}

	return id
}

func benchCtx(b *testing.B) context.Context {
	b.Helper()

	return ctxWithSpanContext(b.Context(), benchTraceID(b), benchSpanID(b), true)
}

// BenchmarkInject measures injecting a trace context into headers.
func BenchmarkInject(b *testing.B) {
	ctx := benchCtx(b)
	headers := map[string]string{"k": "v"}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = Inject(ctx, headers)
	}
}

// BenchmarkExtract measures extracting a trace context from headers.
func BenchmarkExtract(b *testing.B) {
	headers := Inject(benchCtx(b), nil)
	base := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = Extract(base, headers)
	}
}

// BenchmarkTraceID measures reading the trace ID from a span context.
func BenchmarkTraceID(b *testing.B) {
	ctx := benchCtx(b)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		_ = TraceID(ctx)
	}
}

// BenchmarkInjectParallel measures injection under concurrent callers.
func BenchmarkInjectParallel(b *testing.B) {
	ctx := benchCtx(b)
	headers := map[string]string{"k": "v"}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = Inject(ctx, headers)
		}
	})
}
