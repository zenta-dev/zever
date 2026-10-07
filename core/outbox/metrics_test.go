package outbox

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/core/observability"
)

// metricCall records one metric emission.
type metricCall struct {
	kind  string
	name  string
	value float64
	attrs []observability.Attr
}

// fakeMetrics records metric emissions for assertions.
type fakeMetrics struct {
	mu    sync.Mutex
	calls []metricCall
}

func (f *fakeMetrics) Counter(_ context.Context, name string, value float64, attrs ...observability.Attr) error {
	f.add("counter", name, value, attrs)

	return nil
}

func (f *fakeMetrics) Gauge(_ context.Context, name string, value float64, attrs ...observability.Attr) error {
	f.add("gauge", name, value, attrs)

	return nil
}

func (f *fakeMetrics) Histogram(_ context.Context, name string, value float64, attrs ...observability.Attr) error {
	f.add("histogram", name, value, attrs)

	return nil
}

func (f *fakeMetrics) Shutdown(context.Context) error { return nil }

func (f *fakeMetrics) add(kind, name string, value float64, attrs []observability.Attr) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, metricCall{kind: kind, name: name, value: value, attrs: attrs})
}

func (f *fakeMetrics) sum(name string) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()

	var total float64

	for _, c := range f.calls {
		if c.name == name {
			total += c.value
		}
	}

	return total
}

func (f *fakeMetrics) count(kind, name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := 0

	for _, c := range f.calls {
		if c.kind == kind && c.name == name {
			n++
		}
	}

	return n
}

func (f *fakeMetrics) lastGauge(name string) (float64, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i := len(f.calls) - 1; i >= 0; i-- {
		c := f.calls[i]
		if c.kind == "gauge" && c.name == name {
			return c.value, true
		}
	}

	return 0, false
}

func (f *fakeMetrics) attrValues(name, key string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []string

	for _, c := range f.calls {
		if c.name != name {
			continue
		}

		for _, a := range c.attrs {
			if a.Key == key {
				if v, ok := a.Value.(observability.StringValue); ok {
					out = append(out, v.Value)
				}
			}
		}
	}

	return out
}

// spanCall records one ended span.
type spanCall struct {
	name  string
	kind  observability.SpanKind
	attrs []observability.Attr
}

// fakeSpan records its name, kind, and attrs on End.
type fakeSpan struct {
	tracer *fakeTracer
	name   string
	kind   observability.SpanKind
	attrs  []observability.Attr
}

func (s *fakeSpan) SetAttributes(attrs ...observability.Attr) {
	s.attrs = append(s.attrs, attrs...)
}

func (s *fakeSpan) RecordError(error) {}

func (s *fakeSpan) End() {
	s.tracer.mu.Lock()
	defer s.tracer.mu.Unlock()
	s.tracer.spans = append(s.tracer.spans, spanCall{name: s.name, kind: s.kind, attrs: s.attrs})
}

// fakeTracer records ended spans. It implements observability.SpanStarter so
// span start options reach the recorded span.
type fakeTracer struct {
	mu    sync.Mutex
	spans []spanCall
}

func (t *fakeTracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	return ctx, &fakeSpan{tracer: t, name: name}
}

func (t *fakeTracer) StartSpan(
	ctx context.Context,
	name string,
	opts ...observability.SpanStartOption,
) (context.Context, observability.Span) {
	cfg := observability.NewSpanConfig(opts...)

	span := &fakeSpan{tracer: t, name: name, kind: cfg.Kind}
	span.SetAttributes(cfg.Attrs...)

	return ctx, span
}

func (t *fakeTracer) Shutdown(context.Context) error { return nil }

func (t *fakeTracer) spanCount(name string) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	n := 0

	for _, s := range t.spans {
		if s.name == name {
			n++
		}
	}

	return n
}

func (t *fakeTracer) spanAttrs(name string) []observability.Attr {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, s := range t.spans {
		if s.name == name {
			return s.attrs
		}
	}

	return nil
}

func (t *fakeTracer) spanKind(name string) observability.SpanKind {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, s := range t.spans {
		if s.name == name {
			return s.kind
		}
	}

	return observability.SpanKindInternal
}

// fakeProvider implements observability.Provider with recording fakes.
type fakeProvider struct {
	metrics *fakeMetrics
	tracer  *fakeTracer
}

func (p *fakeProvider) Tracer(string) observability.Tracer { return p.tracer }
func (p *fakeProvider) Meter(string) observability.Metrics { return p.metrics }
func (p *fakeProvider) Shutdown(context.Context) error     { return nil }

func newFakeProvider() *fakeProvider {
	return &fakeProvider{metrics: &fakeMetrics{}, tracer: &fakeTracer{}}
}

func TestRecorderEmitsMetrics(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	r := NewRecorder(p, "db", "outbox", "")
	ctx := t.Context()

	r.Status(ctx, 3, 1, 30*time.Second)
	r.Published(ctx)
	r.Retried(ctx)
	r.FailedTotal(ctx)
	r.RelayError(ctx)

	if got := p.metrics.sum(MetricPublished); got != 1 {
		t.Errorf("published = %v, want 1", got)
	}

	if got := p.metrics.sum(MetricRetried); got != 1 {
		t.Errorf("retried = %v, want 1", got)
	}

	if got := p.metrics.sum(MetricFailedTotal); got != 1 {
		t.Errorf("failed_total = %v, want 1", got)
	}

	if got := p.metrics.sum(MetricRelayErrors); got != 1 {
		t.Errorf("relay_errors = %v, want 1", got)
	}

	if got, ok := p.metrics.lastGauge(MetricPending); !ok || got != 3 {
		t.Errorf("pending gauge = %v,%v, want 3,true", got, ok)
	}

	if got, ok := p.metrics.lastGauge(MetricFailed); !ok || got != 1 {
		t.Errorf("failed gauge = %v,%v, want 1,true", got, ok)
	}

	if got, ok := p.metrics.lastGauge(MetricOldestPendingAge); !ok || got != 30 {
		t.Errorf("oldest_pending_age gauge = %v,%v, want 30,true", got, ok)
	}

	if got := p.metrics.attrValues(MetricPublished, AttrOutcome); len(got) != 1 || got[0] != OutcomeOK {
		t.Errorf("published outcome attrs = %v, want [%q]", got, OutcomeOK)
	}

	if got := p.metrics.attrValues(MetricRetried, AttrOutcome); len(got) != 1 || got[0] != OutcomeError {
		t.Errorf("retried outcome attrs = %v, want [%q]", got, OutcomeError)
	}

	if got := p.metrics.attrValues(MetricPending, AttrAdapter); len(got) != 1 || got[0] != "db" {
		t.Errorf("pending adapter attrs = %v, want [db]", got)
	}

	if got := p.metrics.attrValues(MetricPending, AttrTable); len(got) != 1 || got[0] != "outbox" {
		t.Errorf("pending table attrs = %v, want [outbox]", got)
	}
}

func TestRecorderPublishSpan(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	r := NewRecorder(p, "db", "outbox", "")

	spanCtx, finish := r.PublishSpan(t.Context(), "orders", "msg-1")
	if spanCtx == nil {
		t.Fatal("PublishSpan() ctx = nil")
	}

	finish(OutcomeOK)

	if got := p.metrics.count("histogram", MetricPublishDuration); got != 1 {
		t.Errorf("publish duration histogram = %d, want 1", got)
	}

	if got := p.tracer.spanCount(SpanPublish); got != 1 {
		t.Fatalf("publish spans = %d, want 1", got)
	}

	if got := p.tracer.spanKind(SpanPublish); got != observability.SpanKindProducer {
		t.Errorf("publish span kind = %v, want %v", got, observability.SpanKindProducer)
	}

	attrs := p.tracer.spanAttrs(SpanPublish)

	if got := attrStringValue(attrs, AttrTopic); got != "orders" {
		t.Errorf("span topic attr = %q, want orders", got)
	}

	if got := attrStringValue(attrs, AttrAdapter); got != "db" {
		t.Errorf("span adapter attr = %q, want db", got)
	}

	if got := attrStringValue(attrs, AttrTable); got != "outbox" {
		t.Errorf("span table attr = %q, want outbox", got)
	}

	wantMessagingAttrs(t, attrs, "db", "orders", observability.OperationPublish, "msg-1", "")
}

func TestRecorderConsumeSpan(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	r := NewRecorder(p, "cdc", "", "")

	spanCtx, finish := r.ConsumeSpan(t.Context(), "orders", "msg-2")
	if spanCtx == nil {
		t.Fatal("ConsumeSpan() ctx = nil")
	}

	finish()

	if got := p.tracer.spanCount(SpanConsume); got != 1 {
		t.Fatalf("consume spans = %d, want 1", got)
	}

	if got := p.tracer.spanKind(SpanConsume); got != observability.SpanKindConsumer {
		t.Errorf("consume span kind = %v, want %v", got, observability.SpanKindConsumer)
	}

	attrs := p.tracer.spanAttrs(SpanConsume)

	if got := attrStringValue(attrs, AttrTopic); got != "orders" {
		t.Errorf("span topic attr = %q, want orders", got)
	}

	wantMessagingAttrs(t, attrs, "cdc", "orders", observability.OperationProcess, "msg-2", "")
}

// TestRecorderSpanMessagingSystemTransport checks the transport selector wins
// over the adapter name when the recorder was built with one.
func TestRecorderSpanMessagingSystemTransport(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		transport string
		want      string
	}{
		"eventbus": {transport: PublisherEventBus, want: PublisherEventBus},
		"queue":    {transport: PublisherQueue, want: PublisherQueue},
		"unset":    {transport: "", want: "db"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newFakeProvider()
			r := NewRecorder(p, "db", "outbox", tc.transport)

			_, finish := r.PublishSpan(t.Context(), "orders", "msg-1")
			finish(OutcomeOK)

			attrs := p.tracer.spanAttrs(SpanPublish)
			if got := attrStringValue(attrs, observability.MessagingSystem); got != tc.want {
				t.Errorf("messaging.system = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRecorderSpanConversationID checks messaging.message.conversation_id
// carries the trace ID of a context holding a valid span.
func TestRecorderSpanConversationID(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	r := NewRecorder(p, "db", "outbox", "")

	ctx, wantTraceID := contextWithTrace(t)
	_, finish := r.PublishSpan(ctx, "orders", "msg-1")
	finish(OutcomeOK)

	_, finishConsume := r.ConsumeSpan(ctx, "orders", "msg-1")
	finishConsume()

	wantMessagingAttrs(t, p.tracer.spanAttrs(SpanPublish), "db", "orders",
		observability.OperationPublish, "msg-1", wantTraceID)
	wantMessagingAttrs(t, p.tracer.spanAttrs(SpanConsume), "db", "orders",
		observability.OperationProcess, "msg-1", wantTraceID)
}

// TestRecorderSpanOmitsConversationIDWithoutTrace checks a context with no
// valid span leaves messaging.message.conversation_id off the span instead of
// reporting an empty correlation ID.
func TestRecorderSpanOmitsConversationIDWithoutTrace(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	r := NewRecorder(p, "db", "outbox", "")

	_, finish := r.PublishSpan(t.Context(), "billing", "msg-1")
	finish(OutcomeOK)

	_, finishConsume := r.ConsumeSpan(t.Context(), "billing", "msg-1")
	finishConsume()

	// An empty want value asserts the attribute is absent.
	wantMessagingAttrs(t, p.tracer.spanAttrs(SpanPublish), "db", "billing",
		observability.OperationPublish, "msg-1", "")
	wantMessagingAttrs(t, p.tracer.spanAttrs(SpanConsume), "db", "billing",
		observability.OperationProcess, "msg-1", "")
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

func TestRecorderNilProviderNoOp(t *testing.T) {
	t.Parallel()

	r := NewRecorder(nil, "db", "outbox", "")
	ctx := t.Context()

	r.Status(ctx, 1, 2, time.Second)
	r.Published(ctx)
	r.Retried(ctx)
	r.FailedTotal(ctx)
	r.RelayError(ctx)

	spanCtx, finish := r.PublishSpan(ctx, "orders", "msg-1")
	if spanCtx == nil {
		t.Fatal("PublishSpan() ctx = nil")
	}

	finish(OutcomeOK)

	consumeCtx, finishConsume := r.ConsumeSpan(ctx, "orders", "msg-1")
	if consumeCtx == nil {
		t.Fatal("ConsumeSpan() ctx = nil")
	}

	finishConsume()

	// Reaching here without a panic is the assertion: every method no-ops.
}

// attrStringValue returns the string value of key in attrs, or "".
func attrStringValue(attrs []observability.Attr, key string) string {
	for _, a := range attrs {
		if a.Key == key {
			if v, ok := a.Value.(observability.StringValue); ok {
				return v.Value
			}
		}
	}

	return ""
}

// attrHasKey reports whether attrs carries key.
func attrHasKey(attrs []observability.Attr, key string) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}

	return false
}

// wantMessagingAttrs asserts the messaging semantic-convention keys on attrs:
// messaging.system, messaging.destination.name, messaging.operation, and
// messaging.message.id with the given values. An empty conversationID asserts
// messaging.message.conversation_id is absent (the span started over a context
// with no valid trace); otherwise the attribute must carry that trace ID.
func wantMessagingAttrs(
	t *testing.T,
	attrs []observability.Attr,
	system, destination, operation, messageID, conversationID string,
) {
	t.Helper()

	want := map[string]string{
		observability.MessagingSystem:          system,
		observability.MessagingDestinationName: destination,
		observability.MessagingOperation:       operation,
		observability.MessagingMessageID:       messageID,
	}

	for key, value := range want {
		if got := attrStringValue(attrs, key); got != value {
			t.Errorf("span %s = %q, want %q", key, got, value)
		}
	}

	if conversationID == "" {
		if attrHasKey(attrs, observability.MessagingMessageConversationID) {
			t.Errorf("span carries %s, want the attribute omitted", observability.MessagingMessageConversationID)
		}

		return
	}

	if got := attrStringValue(attrs, observability.MessagingMessageConversationID); got != conversationID {
		t.Errorf("span %s = %q, want %q",
			observability.MessagingMessageConversationID, got, conversationID)
	}
}
