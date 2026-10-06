package providersclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v82"
	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/shared/traceprop"
)

func TestNewStripeClient_endpointOverride(t *testing.T) {
	t.Parallel()

	c := NewStripeClient("sk_test", "https://example.com", 0)
	if c == nil {
		t.Fatal("NewStripeClient returned nil")
	}
}

// ctxWithProvidersTrace returns a context carrying a valid remote span.
func ctxWithProvidersTrace(t *testing.T, traceHex string) context.Context {
	t.Helper()

	tid, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}

	sid, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}

	return trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	}))
}

// stripeTraceServer serves one balance response and reports the traceparent
// header the client sent.
func stripeTraceServer(t *testing.T) (endpoint string, seen func() string) {
	t.Helper()

	var got atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get(traceprop.TraceParentHeader))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"balance","available":[{"amount":1,"currency":"usd"}]}`))
	}))
	t.Cleanup(srv.Close)

	return srv.URL, func() string {
		v, _ := got.Load().(string)
		return v
	}
}

func TestNewStripeClientInjectsTraceHeaders(t *testing.T) {
	t.Parallel()

	endpoint, seenTraceparent := stripeTraceServer(t)

	c := NewStripeClient("sk_test", endpoint, time.Second)
	ctx := ctxWithProvidersTrace(t, "4bf92f3577b34da6a3ce929d0e0e4736")
	if _, err := c.V1Balance.Retrieve(ctx, &stripe.BalanceRetrieveParams{}); err != nil {
		t.Fatalf("Balance.Get: %v", err)
	}

	got := seenTraceparent()
	if got == "" {
		t.Fatal("traceparent header missing")
	}

	back := traceprop.Extract(t.Context(), map[string]string{traceprop.TraceParentHeader: got})
	sc := trace.SpanContextFromContext(back)
	if sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("TraceID = %s, want 4bf92f3577b34da6a3ce929d0e0e4736", sc.TraceID())
	}
}

func TestNewStripeClientNoSpanNoTraceHeaders(t *testing.T) {
	t.Parallel()

	endpoint, seenTraceparent := stripeTraceServer(t)

	c := NewStripeClient("sk_test", endpoint, time.Second)
	if _, err := c.V1Balance.Retrieve(t.Context(), &stripe.BalanceRetrieveParams{}); err != nil {
		t.Fatalf("Balance.Get: %v", err)
	}

	if got := seenTraceparent(); got != "" {
		t.Errorf("traceparent = %q, want empty without a span", got)
	}
}
