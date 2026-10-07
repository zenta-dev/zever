package redis

import (
	"context"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/shared/msgspan"
)

// spanCall records one ended messaging span.
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

func (r *spanRecorder) only(t *testing.T) spanCall {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.spans) != 1 {
		t.Fatalf("recorded spans = %d, want 1", len(r.spans))
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

// stubSpanTracer records ended spans and implements observability.SpanStarter,
// so the kind and the start attributes reach the recorded span.
type stubSpanTracer struct {
	rec *spanRecorder
}

func (t *stubSpanTracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	return ctx, &stubSpan{rec: t.rec, name: name}
}

func (t *stubSpanTracer) StartSpan(
	ctx context.Context,
	name string,
	opts ...observability.SpanStartOption,
) (context.Context, observability.Span) {
	cfg := observability.NewSpanConfig(opts...)

	span := &stubSpan{rec: t.rec, name: name, kind: cfg.Kind}
	span.SetAttributes(cfg.Attrs...)

	return ctx, span
}

func (t *stubSpanTracer) Shutdown(context.Context) error { return nil }

// spanProvider hands out one recording tracer.
type spanProvider struct {
	tracer *stubSpanTracer
}

func (p *spanProvider) Tracer(string) observability.Tracer { return p.tracer }

func (p *spanProvider) Meter(string) observability.Metrics { return nil }

func (p *spanProvider) Shutdown(context.Context) error { return nil }

// newSpanProvider returns a Provider backed by a recording tracer plus the
// recorder its spans land in.
func newSpanProvider() (*spanProvider, *spanRecorder) {
	rec := &spanRecorder{}

	return &spanProvider{tracer: &stubSpanTracer{rec: rec}}, rec
}

// wantSpanAttr asserts key is recorded with want.
func wantSpanAttr(t *testing.T, attrs []observability.Attr, key, want string) {
	t.Helper()

	for _, a := range attrs {
		if a.Key != key {
			continue
		}

		v, ok := a.Value.(observability.StringValue)
		if !ok {
			t.Fatalf("span attr %s has a non-string value", key)
		}

		if v.Value != want {
			t.Errorf("span attr %s = %q, want %q", key, v.Value, want)
		}

		return
	}

	t.Errorf("span attrs missing %s: %v", key, attrs)
}

// TestPushProducerSpan verifies Push opens a messaging producer span carrying
// the semantic conventions.
func TestPushProducerSpan(t *testing.T) {
	t.Parallel()

	s := miniredis.RunT(t)
	p, rec := newSpanProvider()

	q, err := New(queue.Options{Addr: s.Addr(), Provider: p})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	t.Cleanup(func() { _ = q.Close() })

	if err := q.Push(t.Context(), "low", queue.Payload("hi"), queue.Headers{"a": "b"}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	call := rec.only(t)

	if call.name != "queue publish low" {
		t.Errorf("span name = %q, want queue publish low", call.name)
	}

	if call.kind != observability.SpanKindProducer {
		t.Errorf("span kind = %v, want producer", call.kind)
	}

	wantSpanAttr(t, call.attrs, observability.MessagingSystem, "queue")
	wantSpanAttr(t, call.attrs, observability.MessagingDestinationName, "low")
	wantSpanAttr(t, call.attrs, observability.MessagingOperation, observability.OperationPublish)
	wantSpanAttr(t, call.attrs, msgspan.AttrOutcome, msgspan.OutcomeOK)
}
