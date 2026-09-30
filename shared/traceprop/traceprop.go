package traceprop

import (
	"context"
	"maps"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TraceParentHeader carries the W3C version-traceid-parentid-flags value.
	TraceParentHeader = "traceparent"
	// TraceStateHeader carries the W3C vendor-specific trace state value.
	TraceStateHeader = "tracestate"
)

// ScopeName identifies the tracer that starts consume spans.
const ScopeName = "github.com/zenta-dev/zever/shared/traceprop"

func propagator() propagation.TraceContext {
	return propagation.TraceContext{}
}

// Inject returns a copy of headers carrying the trace context from ctx.
// It never mutates headers: a nil or headerless input without a valid span
// in ctx comes back unchanged (nil stays nil). When ctx holds no valid span
// context nothing is injected.
func Inject(ctx context.Context, headers map[string]string) map[string]string {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return maps.Clone(headers)
	}

	out := make(map[string]string, len(headers)+2)
	maps.Copy(out, headers)
	propagator().Inject(ctx, propagation.MapCarrier(out))

	return out
}

// Extract returns ctx carrying the remote span context found in headers.
// Empty or invalid headers leave ctx untouched.
func Extract(ctx context.Context, headers map[string]string) context.Context {
	if len(headers) == 0 {
		return ctx
	}

	return propagator().Extract(ctx, propagation.MapCarrier(headers))
}

// StartConsumeSpan extracts the remote span context from headers into ctx
// and starts a consumer child span named spanName over the result.
// Callers must end the returned span (defer span.End()). Extra opts append
// after the default SpanKindConsumer.
func StartConsumeSpan(
	ctx context.Context,
	headers map[string]string,
	spanName string,
	opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	ctx = Extract(ctx, headers)
	opts = append([]trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindConsumer)}, opts...)

	return otel.Tracer(ScopeName).Start(ctx, spanName, opts...)
}
