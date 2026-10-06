package db

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/outbox"
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

// TestRelayPublishSpanMessagingAttrs checks the relay's publish span carries
// the messaging semantic conventions for the message it just published.
func TestRelayPublishSpanMessagingAttrs(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	d := mustNew(t, Options{
		Publisher: &recordingPublisher{},
		Transport: outbox.PublisherEventBus,
		Provider:  p,
	})

	mustRecord(t, d, outbox.Message{ID: "msg-7", Topic: "orders", Payload: []byte("x")})
	d.pollOnce(t.Context())

	attrs := p.tracer.spanAttrs(outbox.SpanPublish)

	if got := p.tracer.spanKind(outbox.SpanPublish); got != observability.SpanKindProducer {
		t.Errorf("publish span kind = %v, want %v", got, observability.SpanKindProducer)
	}

	want := map[string]string{
		observability.MessagingSystem:          outbox.PublisherEventBus,
		observability.MessagingDestinationName: "orders",
		observability.MessagingOperation:       observability.OperationPublish,
		observability.MessagingMessageID:       "msg-7",
	}

	for key, value := range want {
		if got := attrStringValue(attrs, key); got != value {
			t.Errorf("span %s = %q, want %q", key, got, value)
		}
	}

	// The relay ran over a context with no trace, so the correlation ID must
	// be absent rather than empty.
	if attrHasKey(attrs, observability.MessagingMessageConversationID) {
		t.Errorf("span carries %s, want the attribute omitted", observability.MessagingMessageConversationID)
	}
}

// TestRelayPublishSpanMessagingSystemFallsBackToAdapter checks an unset
// transport reports the adapter name as messaging.system.
func TestRelayPublishSpanMessagingSystemFallsBackToAdapter(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	d := mustNew(t, Options{Publisher: &recordingPublisher{}, Provider: p})

	mustRecord(t, d, outbox.Message{ID: "msg-7", Topic: "orders", Payload: []byte("x")})
	d.pollOnce(t.Context())

	attrs := p.tracer.spanAttrs(outbox.SpanPublish)
	if got := attrStringValue(attrs, observability.MessagingSystem); got != string(outbox.DB) {
		t.Errorf("messaging.system = %q, want %q", got, outbox.DB)
	}
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

func TestRelayMetricsPublished(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	p := newFakeProvider()
	d := mustNew(t, Options{Publisher: pub, Provider: p})

	mustRecord(t, d, outbox.Message{ID: "m1", Topic: "orders", Payload: []byte("x")})
	d.pollOnce(t.Context())

	if got := p.metrics.sum(outbox.MetricPublished); got != 1 {
		t.Errorf("published = %v, want 1", got)
	}

	if got := p.metrics.count("histogram", outbox.MetricPublishDuration); got != 1 {
		t.Errorf("publish duration histogram = %d, want 1", got)
	}

	if got := p.tracer.spanCount(outbox.SpanPublish); got != 1 {
		t.Errorf("publish spans = %d, want 1", got)
	}
}

func TestRelayMetricsRetriedThenFailed(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	pub.failNext(100)
	p := newFakeProvider()
	d := mustNew(t, Options{Publisher: pub, MaxAttempts: 2, Provider: p})

	mustRecord(t, d, outbox.Message{ID: "m1", Topic: "t"})

	ctx := t.Context()
	d.pollOnce(ctx)
	d.pollOnce(ctx)

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

func TestRelayMetricsStatusGauges(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	d := mustNew(t, Options{Publisher: &recordingPublisher{}, Provider: p})

	mustRecord(t, d, outbox.Message{ID: "m1", Topic: "t", CreatedAt: time.Now().UTC().Add(-time.Minute)})

	d.Status()

	if got, ok := p.metrics.lastGauge(outbox.MetricPending); !ok || got != 1 {
		t.Errorf("pending gauge = %v,%v, want 1,true", got, ok)
	}

	if got, ok := p.metrics.lastGauge(outbox.MetricFailed); !ok || got != 0 {
		t.Errorf("failed gauge = %v,%v, want 0,true", got, ok)
	}

	if got, ok := p.metrics.lastGauge(outbox.MetricOldestPendingAge); !ok || got <= 0 {
		t.Errorf("oldest_pending_age gauge = %v,%v, want >0,true", got, ok)
	}
}

func TestRelayMetricsGaugesOnPoll(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	d := mustNew(t, Options{Publisher: &recordingPublisher{}, Provider: p})

	mustRecord(t, d, outbox.Message{ID: "m1", Topic: "t"})
	d.pollOnce(t.Context())

	if got := p.metrics.count("gauge", outbox.MetricPending); got == 0 {
		t.Error("pending gauge not emitted on poll")
	}

	if got, ok := p.metrics.lastGauge(outbox.MetricPending); !ok || got != 0 {
		t.Errorf("pending gauge after poll = %v,%v, want 0,true", got, ok)
	}
}

func TestRelayMetricsRelayError(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	d := mustNew(t, Options{Publisher: &recordingPublisher{}, Provider: p})

	d.setRelayError(context.Background(), errors.New("boom"))

	if got := p.metrics.sum(outbox.MetricRelayErrors); got != 1 {
		t.Errorf("relay_errors = %v, want 1", got)
	}
}

func TestRelayMetricsNilProviderNoOp(t *testing.T) {
	t.Parallel()

	pub := &recordingPublisher{}
	d := mustNew(t, Options{Publisher: pub})

	mustRecord(t, d, outbox.Message{ID: "m1", Topic: "t"})
	d.pollOnce(t.Context())
	d.Status()
	d.setRelayError(context.Background(), errors.New("boom"))

	if n := len(pub.messages()); n != 1 {
		t.Fatalf("published %d, want 1", n)
	}
}

func TestRegisterWiresProvider(t *testing.T) {
	t.Parallel()

	Register()
	p := newFakeProvider()

	s, err := outbox.Open(outbox.DB, outbox.Options{DSN: filepath.Join(t.TempDir(), "reg.db"), Provider: p})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	defer func() { _ = s.Close() }()

	d, ok := s.(*driver)
	if !ok {
		t.Fatalf("Open() returned %T, want *driver", s)
	}

	// The Register path carries no Publisher, so the relay cannot publish;
	// Status() still emits gauges through the wired recorder.
	d.Status()

	if got, ok := p.metrics.lastGauge(outbox.MetricPending); !ok || got != 0 {
		t.Errorf("pending gauge = %v,%v, want 0,true", got, ok)
	}
}
