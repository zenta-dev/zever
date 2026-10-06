package msgspan

import (
	"context"
	"strings"
	"sync"
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/core/observability"
)

// spanCall records one ended span.
type spanCall struct {
	name  string
	kind  observability.SpanKind
	attrs []observability.Attr
}

// spanRecorder collects ended spans for assertions.
type spanRecorder struct {
	mu    sync.Mutex
	spans []spanCall
}

func (r *spanRecorder) add(call spanCall) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.spans = append(r.spans, call)
}

func (r *spanRecorder) only(tb testing.TB) spanCall {
	tb.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.spans) != 1 {
		tb.Fatalf("recorded spans = %d, want 1", len(r.spans))
	}

	return r.spans[0]
}

// stubSpan records attributes and reports itself to rec when ended.
type stubSpan struct {
	rec   *spanRecorder
	name  string
	kind  observability.SpanKind
	attrs []observability.Attr
}

func (s *stubSpan) SetAttributes(attrs ...observability.Attr) {
	s.attrs = append(s.attrs, attrs...)
}

func (s *stubSpan) RecordError(error) {}

func (s *stubSpan) End() {
	s.rec.add(spanCall{name: s.name, kind: s.kind, attrs: s.attrs})
}

// stubTracer records ended spans and implements observability.SpanStarter, so
// the span kind and the start attributes reach the recorded span.
type stubTracer struct {
	rec *spanRecorder
}

func (t *stubTracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	return ctx, &stubSpan{rec: t.rec, name: name}
}

func (t *stubTracer) StartSpan(
	ctx context.Context,
	name string,
	opts ...observability.SpanStartOption,
) (context.Context, observability.Span) {
	cfg := observability.NewSpanConfig(opts...)

	span := &stubSpan{rec: t.rec, name: name, kind: cfg.Kind}
	span.SetAttributes(cfg.Attrs...)

	return ctx, span
}

func (t *stubTracer) Shutdown(context.Context) error { return nil }

// plainTracer records ended spans but does NOT implement
// observability.SpanStarter, so observability.StartSpan falls back to Start and
// drops every start option.
type plainTracer struct {
	rec *spanRecorder
}

func (t *plainTracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	return ctx, &stubSpan{rec: t.rec, name: name}
}

func (t *plainTracer) Shutdown(context.Context) error { return nil }

// nilSpanTracer starts spans that resolve to a nil observability.Span.
type nilSpanTracer struct{}

func (nilSpanTracer) Start(context.Context, string) (context.Context, observability.Span) {
	return context.Background(), nil
}

func (nilSpanTracer) Shutdown(context.Context) error { return nil }

// stubProvider hands out one tracer for the scope under test.
type stubProvider struct {
	tracer observability.Tracer
}

func (p *stubProvider) Tracer(string) observability.Tracer { return p.tracer }

func (p *stubProvider) Meter(string) observability.Metrics { return nil }

func (p *stubProvider) Shutdown(context.Context) error { return nil }

// newStubProvider returns a Provider backed by a SpanStarter tracer plus the
// recorder its spans land in.
func newStubProvider() (*stubProvider, *spanRecorder) {
	rec := &spanRecorder{}

	return &stubProvider{tracer: &stubTracer{rec: rec}}, rec
}

// newPlainProvider returns a Provider backed by a tracer without SpanStarter
// plus the recorder its spans land in.
func newPlainProvider() (*stubProvider, *spanRecorder) {
	rec := &spanRecorder{}

	return &stubProvider{tracer: &plainTracer{rec: rec}}, rec
}

// contextWithTrace returns t's context carrying a fixed valid span context and
// its hex trace ID. The IDs are literal, so the test needs no random source
// and no tracing SDK.
func contextWithTrace(t *testing.T) (context.Context, string) {
	t.Helper()

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:     trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		TraceFlags: trace.FlagsSampled,
	})

	if !sc.IsValid() {
		t.Fatal("span context is invalid")
	}

	return trace.ContextWithSpanContext(t.Context(), sc), sc.TraceID().String()
}

// attrValue returns the string value recorded for key, and whether it was
// present.
func attrValue(attrs []observability.Attr, key string) (string, bool) {
	for _, a := range attrs {
		if a.Key != key {
			continue
		}

		v, ok := a.Value.(observability.StringValue)
		if !ok {
			return "", false
		}

		return v.Value, true
	}

	return "", false
}

// wantAttr asserts key is recorded with want.
func wantAttr(t *testing.T, attrs []observability.Attr, key, want string) {
	t.Helper()

	got, ok := attrValue(attrs, key)
	if !ok {
		t.Fatalf("span attrs missing %s: %v", key, attrs)
	}

	if got != want {
		t.Errorf("span attr %s = %q, want %q", key, got, want)
	}
}

// wantNoAttr asserts key is absent from attrs.
func wantNoAttr(t *testing.T, attrs []observability.Attr, key string) {
	t.Helper()

	if got, ok := attrValue(attrs, key); ok {
		t.Errorf("span attr %s = %q, want absent", key, got)
	}
}

func TestProducerSpan(t *testing.T) {
	t.Parallel()

	p, rec := newStubProvider()
	ctx, traceID := contextWithTrace(t)

	spanCtx, finish := Producer(ctx, p, "eventbus", "orders", "msg-1")
	if spanCtx == nil {
		t.Fatal("Producer() ctx = nil")
	}

	finish(OutcomeOK)

	call := rec.only(t)

	if call.name != "eventbus publish orders" {
		t.Errorf("span name = %q, want eventbus publish orders", call.name)
	}

	if call.kind != observability.SpanKindProducer {
		t.Errorf("span kind = %v, want producer", call.kind)
	}

	wantAttr(t, call.attrs, observability.MessagingSystem, "eventbus")
	wantAttr(t, call.attrs, observability.MessagingDestinationName, "orders")
	wantAttr(t, call.attrs, observability.MessagingOperation, observability.OperationPublish)
	wantAttr(t, call.attrs, observability.MessagingMessageID, "msg-1")
	wantAttr(t, call.attrs, observability.MessagingMessageConversationID, traceID)
	wantAttr(t, call.attrs, AttrOutcome, OutcomeOK)
}

func TestConsumerSpan(t *testing.T) {
	t.Parallel()

	p, rec := newStubProvider()
	ctx, traceID := contextWithTrace(t)

	_, finish := Consumer(ctx, p, "queue", "low", "msg-2")
	finish(OutcomeOK)

	call := rec.only(t)

	if call.name != "queue process low" {
		t.Errorf("span name = %q, want queue process low", call.name)
	}

	if call.kind != observability.SpanKindConsumer {
		t.Errorf("span kind = %v, want consumer", call.kind)
	}

	wantAttr(t, call.attrs, observability.MessagingSystem, "queue")
	wantAttr(t, call.attrs, observability.MessagingDestinationName, "low")
	wantAttr(t, call.attrs, observability.MessagingOperation, observability.OperationProcess)
	wantAttr(t, call.attrs, observability.MessagingMessageID, "msg-2")
	wantAttr(t, call.attrs, observability.MessagingMessageConversationID, traceID)
}

// TestSpanNameExcludesMessageID pins the cardinality contract: a message ID
// belongs in an attribute, never in the span name.
func TestSpanNameExcludesMessageID(t *testing.T) {
	t.Parallel()

	p, rec := newStubProvider()

	_, finish := Producer(t.Context(), p, "queue", "high", "msg-3")
	finish(OutcomeOK)

	if name := rec.only(t).name; strings.Contains(name, "msg-3") {
		t.Errorf("span name = %q, want no message ID", name)
	}
}

// TestAttributesSurviveNonSpanStarterTracer covers the fallback path: a Tracer
// without observability.SpanStarter makes StartSpan drop its options, so the
// messaging attributes must survive on the span itself.
func TestAttributesSurviveNonSpanStarterTracer(t *testing.T) {
	t.Parallel()

	p, rec := newPlainProvider()
	ctx, traceID := contextWithTrace(t)

	_, finish := Producer(ctx, p, "queue", "high", "msg-4")
	finish(OutcomeError)

	call := rec.only(t)

	if call.name != "queue publish high" {
		t.Errorf("span name = %q, want queue publish high", call.name)
	}

	wantAttr(t, call.attrs, observability.MessagingSystem, "queue")
	wantAttr(t, call.attrs, observability.MessagingDestinationName, "high")
	wantAttr(t, call.attrs, observability.MessagingOperation, observability.OperationPublish)
	wantAttr(t, call.attrs, observability.MessagingMessageID, "msg-4")
	wantAttr(t, call.attrs, observability.MessagingMessageConversationID, traceID)
	wantAttr(t, call.attrs, AttrOutcome, OutcomeError)
}

// TestOmitsEmptyMessageIDAndTraceID keeps empty correlation IDs off the span.
func TestOmitsEmptyMessageIDAndTraceID(t *testing.T) {
	t.Parallel()

	p, rec := newStubProvider()

	_, finish := Consumer(t.Context(), p, "eventbus", "orders", "")
	finish(OutcomeOK)

	attrs := rec.only(t).attrs

	wantNoAttr(t, attrs, observability.MessagingMessageID)
	wantNoAttr(t, attrs, observability.MessagingMessageConversationID)
	wantAttr(t, attrs, observability.MessagingSystem, "eventbus")
}

// TestFinisherRecordsOutcome checks the outcome attribute across the three
// shapes a caller can pass: ok, error, and empty (recorded as nothing).
func TestFinisherRecordsOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		outcome string
		want    string
	}{
		{name: "ok", outcome: OutcomeOK, want: OutcomeOK},
		{name: "error", outcome: OutcomeError, want: OutcomeError},
		{name: "empty", outcome: "", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p, rec := newStubProvider()

			_, finish := Producer(t.Context(), p, "queue", "low", "msg-5")
			finish(tc.outcome)

			got, ok := attrValue(rec.only(t).attrs, AttrOutcome)
			if tc.want == "" {
				if ok {
					t.Errorf("span attr %s = %q, want absent", AttrOutcome, got)
				}

				return
			}

			if !ok {
				t.Fatalf("span attrs missing %s", AttrOutcome)
			}

			if got != tc.want {
				t.Errorf("span attr %s = %q, want %q", AttrOutcome, got, tc.want)
			}
		})
	}
}

// TestNoopFallbacks covers every unusable input: a nil provider, a provider
// whose tracer is nil, and a tracer whose span is nil. Telemetry must never
// break the publish or delivery around it.
func TestNoopFallbacks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider observability.Provider
	}{
		{name: "nil provider", provider: nil},
		{name: "nil tracer", provider: &stubProvider{tracer: nil}},
		{name: "nil span", provider: &stubProvider{tracer: nilSpanTracer{}}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()

			spanCtx, finish := Producer(ctx, tc.provider, "queue", "low", "msg-6")
			if spanCtx != ctx {
				t.Error("Producer() ctx changed, want the caller's ctx")
			}

			finish(OutcomeOK)
		})
	}
}
