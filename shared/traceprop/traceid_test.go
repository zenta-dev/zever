package traceprop

import (
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestTraceID_validAndEmpty(t *testing.T) {
	t.Parallel()

	if got := TraceID(t.Context()); got != "" {
		t.Errorf("TraceID without span = %q, want empty", got)
	}

	traceID := mustTraceID(t)
	spanID := mustSpanID(t)
	ctx := ctxWithSpanContext(t.Context(), traceID, spanID, true)

	if got := TraceID(ctx); got != traceID.String() {
		t.Errorf("TraceID = %q, want %q", got, traceID.String())
	}

	// Invalid span context yields empty.
	invalid := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{}))
	if got := TraceID(invalid); got != "" {
		t.Errorf("TraceID invalid = %q, want empty", got)
	}
}
