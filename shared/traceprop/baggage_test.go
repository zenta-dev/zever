package traceprop

import (
	"testing"
)

func TestInjectExtract_BaggageRoundTrip(t *testing.T) {
	t.Parallel()

	traceID := mustTraceID(t)
	spanID := mustSpanID(t)

	ctx := ctxWithSpanContext(t.Context(), traceID, spanID, true)
	ctx = WithBaggage(ctx, BaggageTenantID, "acme")
	ctx = WithBaggage(ctx, BaggageUserID, "user-42")

	out := Inject(ctx, nil)

	if out[BaggageHeader] == "" {
		t.Fatalf("baggage header missing in %q", out)
	}

	back := Extract(t.Context(), out)

	if got := Baggage(back, BaggageTenantID); got != "acme" {
		t.Errorf("tenant.id = %q, want %q", got, "acme")
	}

	if got := Baggage(back, BaggageUserID); got != "user-42" {
		t.Errorf("user.id = %q, want %q", got, "user-42")
	}

	if got := Baggage(back, BaggageCorrelationID); got != "" {
		t.Errorf("correlation.id = %q, want empty", got)
	}
}

func TestInject_AllHeadersPresent(t *testing.T) {
	t.Parallel()

	traceID := mustTraceID(t)
	spanID := mustSpanID(t)

	ctx := ctxWithSpanContext(t.Context(), traceID, spanID, true)
	ctx = WithBaggage(ctx, BaggageCorrelationID, "corr-7")

	out := Inject(ctx, nil)

	if out[TraceParentHeader] == "" {
		t.Error("traceparent missing")
	}

	if out[TraceStateHeader] != "" {
		t.Errorf("tracestate = %q, want empty (none set)", out[TraceStateHeader])
	}

	if out[BaggageHeader] == "" {
		t.Error("baggage missing")
	}
}

func TestWithBaggage_InvalidKey(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	if got := WithBaggage(ctx, "bad key!", "v"); got != ctx {
		t.Error("WithBaggage with invalid key should return ctx unchanged")
	}
}

func TestBaggage_Absent(t *testing.T) {
	t.Parallel()

	if got := Baggage(t.Context(), BaggageTenantID); got != "" {
		t.Errorf("Baggage absent = %q, want empty", got)
	}
}
