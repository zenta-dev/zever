package job

import (
	"context"
	"encoding/json/v2"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/adapters/log/noop"
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

// consumeSpan records attributes and reports itself to rec when ended.
type consumeSpan struct {
	rec   *spanRecorder
	name  string
	kind  observability.SpanKind
	attrs []observability.Attr
}

func (s *consumeSpan) SetAttributes(attrs ...observability.Attr) {
	s.attrs = append(s.attrs, attrs...)
}

func (s *consumeSpan) RecordError(error) {}

func (s *consumeSpan) End() {
	s.rec.add(spanCall{name: s.name, kind: s.kind, attrs: s.attrs})
}

// consumeSpanTracer records ended spans and implements
// observability.SpanStarter, so the kind and the start attributes reach the
// recorded span.
type consumeSpanTracer struct {
	rec *spanRecorder
}

func (t *consumeSpanTracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	return ctx, &consumeSpan{rec: t.rec, name: name}
}

func (t *consumeSpanTracer) StartSpan(
	ctx context.Context,
	name string,
	opts ...observability.SpanStartOption,
) (context.Context, observability.Span) {
	cfg := observability.NewSpanConfig(opts...)

	span := &consumeSpan{rec: t.rec, name: name, kind: cfg.Kind}
	span.SetAttributes(cfg.Attrs...)

	return ctx, span
}

func (t *consumeSpanTracer) Shutdown(context.Context) error { return nil }

// consumeSpanProvider hands out one recording tracer.
type consumeSpanProvider struct {
	tracer *consumeSpanTracer
}

func (p *consumeSpanProvider) Tracer(string) observability.Tracer { return p.tracer }

func (p *consumeSpanProvider) Meter(string) observability.Metrics { return nil }

func (p *consumeSpanProvider) Shutdown(context.Context) error { return nil }

// newConsumeSpanProvider returns a Provider backed by a recording tracer plus
// the recorder its spans land in.
func newConsumeSpanProvider() (*consumeSpanProvider, *spanRecorder) {
	rec := &spanRecorder{}

	return &consumeSpanProvider{tracer: &consumeSpanTracer{rec: rec}}, rec
}

// traceIDFromContext returns the hex trace ID carried by ctx, or "" when ctx
// holds no valid span context.
func traceIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}

	return sc.TraceID().String()
}

// mustMessageID parses a fixed UUID literal or fails the test. The IDs are
// literal so the test needs no random source.
func mustMessageID(t *testing.T, id string) queue.MessageID {
	t.Helper()

	parsed, err := queue.ParseMessageID(id)
	if err != nil {
		t.Fatalf("ParseMessageID(%q): %v", id, err)
	}

	return parsed
}

// spanAttr returns the string value recorded for key, and whether it was
// present.
func spanAttr(attrs []observability.Attr, key string) (string, bool) {
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

// wantSpanAttr asserts key is recorded with want.
func wantSpanAttr(t *testing.T, attrs []observability.Attr, key, want string) {
	t.Helper()

	got, ok := spanAttr(attrs, key)
	if !ok {
		t.Fatalf("span attrs missing %s: %v", key, attrs)
	}

	if got != want {
		t.Errorf("span attr %s = %q, want %q", key, got, want)
	}
}

// TestWorkerLaunchHandler_ExtractsTrace verifies the consume-side gap is
// closed: a job message carrying a producer traceparent gives the handler a
// context with the same trace ID, and the drained message is bracketed by a
// messaging consumer span carrying the semantic conventions.
func TestWorkerLaunchHandler_ExtractsTrace(t *testing.T) {
	Reset()

	const traceIDHex = "4bf92f3577b34da6a3ce929d0e0e4736"

	provider, rec := newConsumeSpanProvider()

	gotCh := make(chan string, 1)

	if err := Register("trace-job", func(ctx context.Context, _ string) error {
		gotCh <- traceIDFromContext(ctx)

		return nil
	}); err != nil {
		t.Fatalf("Register trace-job: %v", err)
	}

	payload, _ := json.Marshal("x")
	msg := queue.Message{
		ID:      mustMessageID(t, "0198f0a0-0000-7000-8000-000000000001"),
		Payload: queue.Payload(payload),
		Headers: queue.Headers{
			headerJobName: "trace-job",
			"traceparent": "00-" + traceIDHex + "-00f067aa0ba902b7-01",
		},
		Attempt: 1,
	}

	q := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w := &Worker{Q: q, DrainTimeout: 500 * time.Millisecond, Logger: noop.New(), Provider: provider}

	sem := make(chan struct{}, 1)
	sem <- struct{}{}

	var wg sync.WaitGroup

	w.launchHandler(t.Context(), msg, "low", sem, &wg)
	waitDrain(t, &wg)

	select {
	case got := <-gotCh:
		if got != traceIDHex {
			t.Errorf("handler TraceID = %s, want %s", got, traceIDHex)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called in time")
	}

	call := rec.only(t)

	if call.name != "queue process low" {
		t.Errorf("span name = %q, want queue process low", call.name)
	}

	if call.kind != observability.SpanKindConsumer {
		t.Errorf("span kind = %v, want consumer", call.kind)
	}

	wantSpanAttr(t, call.attrs, observability.MessagingSystem, "queue")
	wantSpanAttr(t, call.attrs, observability.MessagingDestinationName, "low")
	wantSpanAttr(t, call.attrs, observability.MessagingOperation, observability.OperationProcess)
	wantSpanAttr(t, call.attrs, observability.MessagingMessageID, msg.ID.String())
	wantSpanAttr(t, call.attrs, observability.MessagingMessageConversationID, traceIDHex)
	wantSpanAttr(t, call.attrs, msgspan.AttrOutcome, "ok")
}

// TestWorkerLaunchHandler_SpanRecordsHandlerError checks a failing handler
// records the error outcome on the consumer span.
func TestWorkerLaunchHandler_SpanRecordsHandlerError(t *testing.T) {
	Reset()

	provider, rec := newConsumeSpanProvider()

	if err := Register("failing-job", func(context.Context, string) error {
		return context.Canceled
	}); err != nil {
		t.Fatalf("Register failing-job: %v", err)
	}

	payload, _ := json.Marshal("x")
	msg := queue.Message{
		ID:      mustMessageID(t, "0198f0a0-0000-7000-8000-000000000002"),
		Topic:   "low",
		Payload: queue.Payload(payload),
		Headers: queue.Headers{headerJobName: "failing-job"},
		Attempt: 1,
	}

	q := &workerStubQueue{
		ackFn:         func(context.Context, queue.Message) error { return nil },
		pushDelayedFn: func(context.Context, string, queue.Payload, queue.Headers, time.Duration) error { return nil },
	}
	w := &Worker{Q: q, DrainTimeout: 500 * time.Millisecond, Logger: noop.New(), Provider: provider}

	sem := make(chan struct{}, 1)
	sem <- struct{}{}

	var wg sync.WaitGroup

	w.launchHandler(t.Context(), msg, "low", sem, &wg)
	waitDrain(t, &wg)

	wantSpanAttr(t, rec.only(t).attrs, msgspan.AttrOutcome, "error")
}

// TestWorkerLaunchHandler_NoProvider checks the drain path stays silent (and
// keeps extracting the producer trace) with no observability provider wired.
func TestWorkerLaunchHandler_NoProvider(t *testing.T) {
	Reset()

	const traceIDHex = "4bf92f3577b34da6a3ce929d0e0e4736"

	gotCh := make(chan string, 1)

	if err := Register("untraced-provider-job", func(ctx context.Context, _ string) error {
		gotCh <- traceIDFromContext(ctx)

		return nil
	}); err != nil {
		t.Fatalf("Register untraced-provider-job: %v", err)
	}

	payload, _ := json.Marshal("x")
	msg := queue.Message{
		ID:      mustMessageID(t, "0198f0a0-0000-7000-8000-000000000003"),
		Payload: queue.Payload(payload),
		Headers: queue.Headers{
			headerJobName: "untraced-provider-job",
			"traceparent": "00-" + traceIDHex + "-00f067aa0ba902b7-01",
		},
		Attempt: 1,
	}

	q := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w := &Worker{Q: q, DrainTimeout: 500 * time.Millisecond, Logger: noop.New()}

	sem := make(chan struct{}, 1)
	sem <- struct{}{}

	var wg sync.WaitGroup

	w.launchHandler(t.Context(), msg, "low", sem, &wg)
	waitDrain(t, &wg)

	select {
	case got := <-gotCh:
		if got != traceIDHex {
			t.Errorf("handler TraceID = %s, want %s", got, traceIDHex)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called in time")
	}
}

// waitDrain blocks until the handler goroutine launched by launchHandler
// returns, mirroring the shutdown drain the worker performs.
func waitDrain(t *testing.T, inflight *sync.WaitGroup) {
	t.Helper()

	done := make(chan struct{})

	go func() {
		inflight.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("launchHandler didn't complete")
	}
}
