package http

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/shared/traceprop"
)

func ctxWithTraceID(t *testing.T, traceHex, spanHex string) context.Context {
	t.Helper()

	tid, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}
	sid, err := trace.SpanIDFromHex(spanHex)
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}

	return trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	}))
}

func TestDeliverInjectsTraceHeaders(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var gotParent, gotState string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotParent = r.Header.Get("traceparent")
		gotState = r.Header.Get("tracestate")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)
	payload := []byte(`{"a":1}`)
	if err := w.Register(t.Context(), "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	ctx := ctxWithTraceID(t, "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7")
	if err := w.Deliver(ctx, "e", payload); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotParent == "" {
		t.Fatal("traceparent header missing")
	}
	back := traceprop.Extract(t.Context(), map[string]string{
		traceprop.TraceParentHeader: gotParent,
		traceprop.TraceStateHeader:  gotState,
	})
	sc := trace.SpanContextFromContext(back)
	if !sc.IsValid() {
		t.Fatalf("extracted span invalid from %q", gotParent)
	}
	if sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("TraceID = %s, want 4bf92f3577b34da6a3ce929d0e0e4736", sc.TraceID())
	}
}

func TestDeliverNoSpanNoTraceHeaders(t *testing.T) {
	t.Parallel()

	var gotParent string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = body
		mu.Lock()
		defer mu.Unlock()
		gotParent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openPrivate(t, 1)
	if err := w.Register(t.Context(), "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	if err := w.Deliver(t.Context(), "e", []byte(`{}`)); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotParent != "" {
		t.Errorf("traceparent = %q, want empty without span", gotParent)
	}
}
