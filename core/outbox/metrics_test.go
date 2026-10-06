package outbox

import (
	"context"
	"sync"
	"testing"
	"time"

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

func TestRecorderEmitsMetrics(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	r := NewRecorder(p, "db", "outbox")
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
	r := NewRecorder(p, "db", "outbox")

	spanCtx, finish := r.PublishSpan(t.Context(), "orders")
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
}

func TestRecorderConsumeSpan(t *testing.T) {
	t.Parallel()

	p := newFakeProvider()
	r := NewRecorder(p, "cdc", "")

	spanCtx, finish := r.ConsumeSpan(t.Context(), "orders")
	if spanCtx == nil {
		t.Fatal("ConsumeSpan() ctx = nil")
	}

	finish()

	if got := p.tracer.spanCount(SpanConsume); got != 1 {
		t.Fatalf("consume spans = %d, want 1", got)
	}

	if got := attrStringValue(p.tracer.spanAttrs(SpanConsume), AttrTopic); got != "orders" {
		t.Errorf("span topic attr = %q, want orders", got)
	}
}

func TestRecorderNilProviderNoOp(t *testing.T) {
	t.Parallel()

	r := NewRecorder(nil, "db", "outbox")
	ctx := t.Context()

	r.Status(ctx, 1, 2, time.Second)
	r.Published(ctx)
	r.Retried(ctx)
	r.FailedTotal(ctx)
	r.RelayError(ctx)

	spanCtx, finish := r.PublishSpan(ctx, "orders")
	if spanCtx == nil {
		t.Fatal("PublishSpan() ctx = nil")
	}

	finish(OutcomeOK)

	consumeCtx, finishConsume := r.ConsumeSpan(ctx, "orders")
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
