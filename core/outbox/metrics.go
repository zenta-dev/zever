package outbox

import (
	"context"
	"time"

	"github.com/zenta-dev/zever/core/observability"
)

// Metric names emitted by the outbox relay. Every name carries the "outbox."
// prefix so backends namespace relay telemetry under the outbox scope.
const (
	// MetricPending is the gauge of messages awaiting publication.
	MetricPending = "outbox.pending"
	// MetricFailed is the gauge of messages that exhausted MaxAttempts.
	MetricFailed = "outbox.failed"
	// MetricOldestPendingAge is the gauge of the oldest pending message's
	// age in seconds; 0 when no message is pending.
	MetricOldestPendingAge = "outbox.oldest_pending_age_seconds"
	// MetricPublished counts successful publishes.
	MetricPublished = "outbox.published"
	// MetricFailedTotal counts messages moved to the failed state.
	MetricFailedTotal = "outbox.failed_total"
	// MetricRetried counts publish failures that will be retried.
	MetricRetried = "outbox.retried"
	// MetricRelayErrors counts relay loop errors.
	MetricRelayErrors = "outbox.relay_errors"
	// MetricPublishDuration is the histogram of publish durations in seconds.
	MetricPublishDuration = "outbox.publish.duration_seconds"
)

// Attribute keys attached to outbox relay metrics and spans.
const (
	// AttrAdapter is the adapter name ("db", "cdc").
	AttrAdapter = "outbox.adapter"
	// AttrTable is the outbox table name (db adapter only).
	AttrTable = "outbox.table"
	// AttrTopic is the message topic.
	AttrTopic = "outbox.topic"
	// AttrOutcome is the publish outcome: OutcomeOK or OutcomeError.
	AttrOutcome = "outbox.outcome"
)

// Outcome values for AttrOutcome.
const (
	// OutcomeOK marks a successful publish.
	OutcomeOK = "ok"
	// OutcomeError marks a failed publish.
	OutcomeError = "error"
)

// Span names started by the outbox relay.
const (
	// SpanPublish is the per-publish span started by the db relay.
	SpanPublish = "outbox.publish"
	// SpanConsume is the per-message span started by the cdc consumer.
	SpanConsume = "outbox.consume"
)

// Recorder emits outbox relay metrics and spans. It is best-effort: a nil
// Provider makes every method a no-op, and metric or span errors are
// discarded so telemetry never fails the relay.
type Recorder struct {
	provider observability.Provider
	adapter  string
	table    string
}

// NewRecorder returns a Recorder bound to provider and tagged with adapter
// and table. A nil provider yields a no-op Recorder.
func NewRecorder(provider observability.Provider, adapter, table string) Recorder {
	return Recorder{provider: provider, adapter: adapter, table: table}
}

// Status emits the pending, failed, and oldest-pending-age gauges.
func (r Recorder) Status(ctx context.Context, pending, failed int64, oldestAge time.Duration) {
	m := r.meter()
	if m == nil {
		return
	}

	attrs := r.attrs()
	_ = m.Gauge(ctx, MetricPending, float64(pending), attrs...)
	_ = m.Gauge(ctx, MetricFailed, float64(failed), attrs...)
	_ = m.Gauge(ctx, MetricOldestPendingAge, oldestAge.Seconds(), attrs...)
}

// Published emits the published counter with outcome ok.
func (r Recorder) Published(ctx context.Context) {
	r.counter(ctx, MetricPublished, observability.String(AttrOutcome, OutcomeOK))
}

// Retried emits the retried counter with outcome error.
func (r Recorder) Retried(ctx context.Context) {
	r.counter(ctx, MetricRetried, observability.String(AttrOutcome, OutcomeError))
}

// FailedTotal emits the failed_total counter with outcome error.
func (r Recorder) FailedTotal(ctx context.Context) {
	r.counter(ctx, MetricFailedTotal, observability.String(AttrOutcome, OutcomeError))
}

// RelayError emits the relay_errors counter with outcome error.
func (r Recorder) RelayError(ctx context.Context) {
	r.counter(ctx, MetricRelayErrors, observability.String(AttrOutcome, OutcomeError))
}

// PublishSpan starts an outbox.publish span for topic and returns a
// finisher that ends the span and records the publish duration histogram
// with the given outcome. The finisher is a no-op when no provider is wired.
func (r Recorder) PublishSpan(ctx context.Context, topic string) (context.Context, func(outcome string)) {
	t := r.tracer()
	if t == nil {
		return ctx, func(string) {}
	}

	spanCtx, span := t.Start(ctx, SpanPublish)
	span.SetAttributes(r.spanAttrs(topic)...)

	start := time.Now()

	return spanCtx, func(outcome string) {
		span.End()

		m := r.meter()
		if m == nil {
			return
		}

		_ = m.Histogram(spanCtx, MetricPublishDuration, time.Since(start).Seconds(),
			r.attrs(observability.String(AttrOutcome, outcome))...)
	}
}

// ConsumeSpan starts an outbox.consume span for topic and returns a
// finisher that ends the span. The finisher is a no-op when no provider is
// wired.
func (r Recorder) ConsumeSpan(ctx context.Context, topic string) (context.Context, func()) {
	t := r.tracer()
	if t == nil {
		return ctx, func() {}
	}

	spanCtx, span := t.Start(ctx, SpanConsume)
	span.SetAttributes(r.spanAttrs(topic)...)

	return spanCtx, span.End
}

// counter adds 1 to the named counter with the base attrs plus extra.
func (r Recorder) counter(ctx context.Context, name string, extra ...observability.Attr) {
	m := r.meter()
	if m == nil {
		return
	}

	_ = m.Counter(ctx, name, 1, r.attrs(extra...)...)
}

// meter returns the scope meter, or nil when no provider is wired.
func (r Recorder) meter() observability.Metrics {
	if r.provider == nil {
		return nil
	}

	return r.provider.Meter(ScopeName)
}

// tracer returns the scope tracer, or nil when no provider is wired.
func (r Recorder) tracer() observability.Tracer {
	if r.provider == nil {
		return nil
	}

	return r.provider.Tracer(ScopeName)
}

// attrs returns the base metric attrs: adapter and, when set, table.
func (r Recorder) attrs(extra ...observability.Attr) []observability.Attr {
	out := make([]observability.Attr, 0, len(extra)+2)
	out = append(out, observability.String(AttrAdapter, r.adapter))

	if r.table != "" {
		out = append(out, observability.String(AttrTable, r.table))
	}

	return append(out, extra...)
}

// spanAttrs returns the span attrs: topic, adapter and, when set, table.
func (r Recorder) spanAttrs(topic string) []observability.Attr {
	out := []observability.Attr{
		observability.String(AttrTopic, topic),
		observability.String(AttrAdapter, r.adapter),
	}

	if r.table != "" {
		out = append(out, observability.String(AttrTable, r.table))
	}

	return out
}
