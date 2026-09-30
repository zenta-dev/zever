package traceprop

import (
	"testing"

	"go.opentelemetry.io/otel"
)

// TestExtractOrBackground_Valid verifies a pushed span context survives the
// Push/Pop header round trip: extraction yields the same trace ID as remote.
func TestExtractOrBackground_Valid(t *testing.T) {
	t.Parallel()

	traceID := mustTraceID(t)
	spanID := mustSpanID(t)

	headers := Inject(
		ctxWithSpanContext(t.Context(), traceID, spanID, true),
		map[string]string{"k": "v"},
	)

	back := spanContextOf(ExtractOrBackground(headers))
	if back.TraceID() != traceID {
		t.Errorf("TraceID = %s, want %s", back.TraceID(), traceID)
	}
	if !back.IsRemote() {
		t.Error("IsRemote = false, want true (extracted context is remote)")
	}
}

// TestExtractOrBackground_Untrusted verifies missing or invalid headers fall
// back to a background context with no valid span instead of propagating junk.
func TestExtractOrBackground_Untrusted(t *testing.T) {
	t.Parallel()

	for _, headers := range []map[string]string{nil, {}, {"traceparent": "bogus"}} {
		if back := spanContextOf(ExtractOrBackground(headers)); back.IsValid() {
			t.Errorf("ExtractOrBackground(%q) valid = %s, want invalid", headers, back.TraceID())
		}
	}
}

// TestContinueSpan_LinksRemoteParent verifies ContinueSpan extracts then
// starts a consumer span parented on the propagated remote context.
func TestContinueSpan_LinksRemoteParent(t *testing.T) {
	t.Parallel()

	prev := otel.GetTracerProvider()
	st := &stubTracer{}
	otel.SetTracerProvider(&stubProvider{tracer: st})
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	traceID := mustTraceID(t)
	spanID := mustSpanID(t)

	headers := Inject(ctxWithSpanContext(t.Context(), traceID, spanID, true), nil)

	ctx, span := ContinueSpan(t.Context(), headers, "test.continue")
	defer span.End()

	stub, ok := span.(*stubSpan)
	if !ok {
		t.Fatalf("span = %T, want *stubSpan", span)
	}

	if stub.parent.TraceID() != traceID || stub.parent.SpanID() != spanID {
		t.Errorf("parent = %s/%s, want %s/%s", stub.parent.TraceID(), stub.parent.SpanID(), traceID, spanID)
	}

	if got := spanContextOf(ctx); !got.IsValid() {
		t.Error("returned ctx holds invalid span context")
	}
}

// TestContinueSpan_InvalidHeaders verifies ContinueSpan still returns a
// usable context and span when headers carry nothing extractable.
func TestContinueSpan_InvalidHeaders(t *testing.T) {
	t.Parallel()

	ctx, span := ContinueSpan(t.Context(), map[string]string{"traceparent": "bogus"}, "test.continue-invalid")
	defer span.End()

	if ctx == nil {
		t.Error("ContinueSpan returned nil ctx for invalid headers")
	}
}
