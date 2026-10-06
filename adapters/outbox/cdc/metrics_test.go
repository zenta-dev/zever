package cdc

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/shared/retry"
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

func TestConsumeMetricsPublished(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	s := &store{
		prefix:      DefaultPrefix,
		maxAttempts: 3,
		sleep:       recordingSleep(new([]time.Duration)),
		recorder:    outbox.NewRecorder(p, "cdc", "", ""),
	}

	pub := &stubPublisher{}
	msg := outbox.Message{ID: "evt-1", Topic: "orders", Payload: []byte("p")}

	advanced, err := s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), pub.Publish)
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}

	if !advanced {
		t.Error("handleMessage() advanced = false, want true")
	}

	if got := p.metrics.sum(outbox.MetricPublished); got != 1 {
		t.Errorf("published = %v, want 1", got)
	}

	if got := p.tracer.spanCount(outbox.SpanConsume); got != 1 {
		t.Fatalf("consume spans = %d, want 1", got)
	}

	if got := attrStringValue(p.tracer.spanAttrs(outbox.SpanConsume), outbox.AttrTopic); got != "orders" {
		t.Errorf("span topic attr = %q, want orders", got)
	}
}

// TestConsumeSpanMessagingAttrs checks the consumer's consume span carries the
// messaging semantic conventions for the message it handled.
func TestConsumeSpanMessagingAttrs(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	s := &store{
		prefix:      DefaultPrefix,
		maxAttempts: 3,
		sleep:       recordingSleep(new([]time.Duration)),
		recorder:    outbox.NewRecorder(p, "cdc", "", outbox.PublisherQueue),
	}

	pub := &stubPublisher{}
	msg := outbox.Message{ID: "evt-9", Topic: "orders", Payload: []byte("p")}

	if _, err := s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), pub.Publish); err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}

	attrs := p.tracer.spanAttrs(outbox.SpanConsume)

	if got := p.tracer.spanKind(outbox.SpanConsume); got != observability.SpanKindConsumer {
		t.Errorf("consume span kind = %v, want %v", got, observability.SpanKindConsumer)
	}

	want := map[string]string{
		observability.MessagingSystem:          outbox.PublisherQueue,
		observability.MessagingDestinationName: "orders",
		observability.MessagingOperation:       observability.OperationProcess,
		observability.MessagingMessageID:       "evt-9",
	}

	for key, value := range want {
		if got := attrStringValue(attrs, key); got != value {
			t.Errorf("span %s = %q, want %q", key, got, value)
		}
	}

	// The consumer ran over a context with no trace, so the correlation ID
	// must be absent rather than empty.
	if attrHasKey(attrs, observability.MessagingMessageConversationID) {
		t.Errorf("span carries %s, want the attribute omitted", observability.MessagingMessageConversationID)
	}
}

// TestConsumeSpanMessagingSystemFallsBackToAdapter checks an unset transport
// reports the adapter name as messaging.system.
func TestConsumeSpanMessagingSystemFallsBackToAdapter(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	s := &store{
		prefix:      DefaultPrefix,
		maxAttempts: 3,
		sleep:       recordingSleep(new([]time.Duration)),
		recorder:    outbox.NewRecorder(p, "cdc", "", ""),
	}

	pub := &stubPublisher{}
	msg := outbox.Message{ID: "evt-9", Topic: "orders", Payload: []byte("p")}

	if _, err := s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), pub.Publish); err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}

	attrs := p.tracer.spanAttrs(outbox.SpanConsume)
	if got := attrStringValue(attrs, observability.MessagingSystem); got != string(outbox.CDC) {
		t.Errorf("messaging.system = %q, want %q", got, outbox.CDC)
	}
}

func TestConsumeMetricsRetriedThenFailed(t *testing.T) {
	t.Parallel()

	delays := []time.Duration{}
	p := newFakeProvider()
	s := &store{
		prefix:      DefaultPrefix,
		maxAttempts: 2,
		retry:       retry.Policy{BaseDelay: time.Millisecond},
		sleep:       recordingSleep(&delays),
		recorder:    outbox.NewRecorder(p, "cdc", "", ""),
	}

	alwaysFail := func(context.Context, outbox.Message) error {
		return errors.New("always fail")
	}

	msg := outbox.Message{ID: "evt-1", Topic: "orders"}

	advanced, err := s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), alwaysFail)
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}

	if advanced {
		t.Error("handleMessage() advanced = true, want false after exhausted retries")
	}

	if got := p.metrics.sum(outbox.MetricRetried); got != 1 {
		t.Errorf("retried = %v, want 1", got)
	}

	if got := p.metrics.sum(outbox.MetricFailedTotal); got != 1 {
		t.Errorf("failed_total = %v, want 1", got)
	}

	if got := p.metrics.sum(outbox.MetricPublished); got != 0 {
		t.Errorf("published = %v, want 0", got)
	}
}

func TestConsumeMetricsRelayError(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	s := &store{
		prefix:      DefaultPrefix,
		maxAttempts: 3,
		sleep:       recordingSleep(new([]time.Duration)),
		recorder:    outbox.NewRecorder(p, "cdc", "", ""),
	}

	s.setRelayError(context.Background(), errors.New("boom"))

	if got := p.metrics.sum(outbox.MetricRelayErrors); got != 1 {
		t.Errorf("relay_errors = %v, want 1", got)
	}
}

func TestConsumeMetricsNilProviderNoOp(t *testing.T) {
	t.Parallel()

	s := &store{prefix: DefaultPrefix, maxAttempts: 3, sleep: recordingSleep(new([]time.Duration))}
	pub := &stubPublisher{}
	msg := outbox.Message{ID: "evt-1", Topic: "orders"}

	advanced, err := s.handleMessage(t.Context(), DefaultPrefix, mustPayload(t, msg), pub.Publish)
	if err != nil {
		t.Fatalf("handleMessage() error = %v", err)
	}

	if !advanced {
		t.Error("handleMessage() advanced = false, want true")
	}

	if pub.count() != 1 {
		t.Errorf("published = %d, want 1", pub.count())
	}
}
