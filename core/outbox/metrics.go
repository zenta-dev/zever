package outbox

import (
	"context"
	"time"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/shared/traceprop"
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
	provider  observability.Provider
	adapter   string
	table     string
	transport string
}

// NewRecorder returns a Recorder bound to provider and tagged with adapter,
// table, and transport. A nil provider yields a no-op Recorder. transport is
// the messaging transport selector (outbox.PublisherEventBus,
// outbox.PublisherQueue); empty falls back to the adapter name for the
// messaging.system span attribute.
func NewRecorder(provider observability.Provider, adapter, table, transport string) Recorder {
	return Recorder{provider: provider, adapter: adapter, table: table, transport: transport}
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

// PublishSpan starts an outbox.publish producer span for the message with the
// given topic and ID and returns a finisher that ends the span and records the
// publish duration histogram with the given outcome. The span carries the
// outbox.* attrs plus the messaging semantic-convention set: messaging.system,
// messaging.destination.name (the topic), messaging.operation (publish),
// messaging.message.id, and messaging.message.conversation_id when ctx holds a
// trace. The finisher is a no-op when no provider is wired.
func (r Recorder) PublishSpan(ctx context.Context, topic, messageID string) (context.Context, func(outcome string)) {
	t := r.tracer()
	if t == nil {
		return ctx, func(string) {}
	}

	start := time.Now()

	spanCtx, span := observability.StartSpan(ctx, t, SpanPublish,
		observability.WithSpanKind(observability.SpanKindProducer),
		observability.WithAttributes(r.spanAttrs(ctx, topic, messageID, observability.OperationPublish)...),
	)

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

// ConsumeSpan starts an outbox.consume consumer span for the message with the
// given topic and ID and returns a finisher that ends the span. The span
// carries the same outbox.* and messaging.* attributes as PublishSpan, with
// messaging.operation set to process. The finisher is a no-op when no provider
// is wired.
func (r Recorder) ConsumeSpan(ctx context.Context, topic, messageID string) (context.Context, func()) {
	t := r.tracer()
	if t == nil {
		return ctx, func() {}
	}

	spanCtx, span := observability.StartSpan(ctx, t, SpanConsume,
		observability.WithSpanKind(observability.SpanKindConsumer),
		observability.WithAttributes(r.spanAttrs(ctx, topic, messageID, observability.OperationProcess)...),
	)

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

// system returns the messaging.system value: the configured transport
// selector when set, otherwise the adapter name.
func (r Recorder) system() string {
	if r.transport != "" {
		return r.transport
	}

	return r.adapter
}

// spanAttrs returns the span attrs for one message: the outbox.* set (topic,
// adapter, and, when set, table) followed by the messaging.* semantic
// conventions. The conversation ID is omitted when ctx carries no valid trace,
// so the attribute never reports an empty correlation ID.
func (r Recorder) spanAttrs(ctx context.Context, topic, messageID, operation string) []observability.Attr {
	out := make([]observability.Attr, 0, 8)
	out = append(out,
		observability.String(AttrTopic, topic),
		observability.String(AttrAdapter, r.adapter),
	)

	if r.table != "" {
		out = append(out, observability.String(AttrTable, r.table))
	}

	out = append(out,
		observability.String(observability.MessagingSystem, r.system()),
		observability.String(observability.MessagingDestinationName, topic),
		observability.String(observability.MessagingOperation, operation),
		observability.String(observability.MessagingMessageID, messageID),
	)

	if traceID := traceprop.TraceID(ctx); traceID != "" {
		out = append(out, observability.String(observability.MessagingMessageConversationID, traceID))
	}

	return out
}
