package traceprop

import (
	"context"
	"maps"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	// TraceParentHeader carries the W3C version-traceid-parentid-flags value.
	TraceParentHeader = "traceparent"
	// TraceStateHeader carries the W3C vendor-specific trace state value.
	TraceStateHeader = "tracestate"
	// BaggageHeader carries the W3C baggage value.
	BaggageHeader = "baggage"
)

// Well-known baggage keys propagated across service boundaries.
const (
	// BaggageTenantID is the baggage key for the tenant identifier.
	BaggageTenantID = "tenant.id"
	// BaggageUserID is the baggage key for the user identifier.
	BaggageUserID = "user.id"
	// BaggageCorrelationID is the baggage key for the correlation identifier.
	BaggageCorrelationID = "correlation.id"
)

// ScopeName identifies the tracer that starts consume spans.
const ScopeName = "github.com/zenta-dev/zever/shared/traceprop"

func propagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// Inject returns a copy of headers carrying the trace context and baggage
// from ctx. It never mutates headers: a nil or headerless input without a
// valid span in ctx comes back unchanged (nil stays nil). When ctx holds no
// valid span context nothing is injected.
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

// ExtractOrBackground returns the remote span context found in headers over
// a fresh background context. Missing or invalid headers yield a background
// context with no valid span. Incoming propagation is untrusted: the OTel
// propagator validates traceparent/tracestate and invalid values are a no-op.
func ExtractOrBackground(headers map[string]string) context.Context {
	return Extract(context.Background(), headers)
}

// WithBaggage returns a context carrying the baggage key/value pair, merged
// with any baggage already on ctx. An invalid key or value leaves ctx
// unchanged.
func WithBaggage(ctx context.Context, key, val string) context.Context {
	m, err := baggage.NewMember(key, val)
	if err != nil {
		return ctx
	}

	b, err := baggage.FromContext(ctx).SetMember(m)
	if err != nil {
		return ctx
	}

	return baggage.ContextWithBaggage(ctx, b)
}

// Baggage returns the baggage value for key from ctx, or "" when absent.
func Baggage(ctx context.Context, key string) string {
	return baggage.FromContext(ctx).Member(key).Value()
}

// ContinueSpan extracts the remote span context from headers into ctx and
// starts a consumer child span named spanName over the result. It is
// StartConsumeSpan in one call for consume paths that already hold a
// caller context. Callers must end the returned span (defer span.End()).
// Extra opts append after the default SpanKindConsumer.
//
//nolint:spancheck // ContinueSpan returns the span to the caller, who owns calling End() (documented above); this is the standard tracer.Start contract, not a leak
func ContinueSpan(
	ctx context.Context,
	headers map[string]string,
	spanName string,
	opts ...trace.SpanStartOption,
) (context.Context, trace.Span) {
	return StartConsumeSpan(ctx, headers, spanName, opts...)
}

// TraceID returns the hex-encoded trace ID from ctx, or "" when ctx holds
// no valid span. It is the log/metadata correlation escape hatch for send
// paths with no header channel (mailer, SMS/push): record the ID in logs,
// never invent protocol headers.
func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}

	return sc.TraceID().String()
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

	//nolint:spancheck // StartConsumeSpan returns the span to the caller, who owns calling End(); standard tracer.Start contract, not a leak
	return otel.Tracer(ScopeName).Start(ctx, spanName, opts...)
}
