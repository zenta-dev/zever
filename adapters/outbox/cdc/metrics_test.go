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
	attrs []observability.Attr
}

// fakeSpan records its name and attrs on End.
type fakeSpan struct {
	tracer *fakeTracer
	name   string
	attrs  []observability.Attr
}

func (s *fakeSpan) SetAttributes(attrs ...observability.Attr) {
	s.attrs = append(s.attrs, attrs...)
}

func (s *fakeSpan) RecordError(error) {}

func (s *fakeSpan) End() {
	s.tracer.mu.Lock()
	defer s.tracer.mu.Unlock()
	s.tracer.spans = append(s.tracer.spans, spanCall{name: s.name, attrs: s.attrs})
}

// fakeTracer records ended spans.
type fakeTracer struct {
	mu    sync.Mutex
	spans []spanCall
}

func (t *fakeTracer) Start(ctx context.Context, name string) (context.Context, observability.Span) {
	return ctx, &fakeSpan{tracer: t, name: name}
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

func TestConsumeMetricsPublished(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	s := &store{
		prefix:      DefaultPrefix,
		maxAttempts: 3,
		sleep:       recordingSleep(new([]time.Duration)),
		recorder:    outbox.NewRecorder(p, "cdc", ""),
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

func TestConsumeMetricsRetriedThenFailed(t *testing.T) {
	t.Parallel()

	delays := []time.Duration{}
	p := newFakeProvider()
	s := &store{
		prefix:      DefaultPrefix,
		maxAttempts: 2,
		retry:       retry.Policy{BaseDelay: time.Millisecond},
		sleep:       recordingSleep(&delays),
		recorder:    outbox.NewRecorder(p, "cdc", ""),
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
		recorder:    outbox.NewRecorder(p, "cdc", ""),
	}

	s.setRelayError(errors.New("boom"))

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
