package msgspan

import (
	"context"

	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/shared/traceprop"
)

// ScopeName identifies the tracer that records messaging spans.
const ScopeName = "github.com/zenta-dev/zever/shared/msgspan"

// AttrOutcome is the span attribute carrying the outcome passed to Finisher.
const AttrOutcome = "messaging.outcome"

// Outcome values recorded in AttrOutcome.
const (
	// OutcomeOK marks a successful publish or delivery.
	OutcomeOK = "ok"
	// OutcomeError marks a failed publish or delivery.
	OutcomeError = "error"
)

// Finisher ends a messaging span. The outcome ("ok", "error", or a caller
// chosen label) is recorded as AttrOutcome; an empty outcome records nothing.
// Finisher is safe to call once, and a no-op Finisher is returned when no
// telemetry is wired.
type Finisher func(outcome string)

// Producer starts a producer span for a message published to destination. It
// returns the span context and a Finisher ending the span. A nil provider, a
// nil tracer, or a nil span yields ctx unchanged and a no-op Finisher.
func Producer(ctx context.Context, p observability.Provider, system, destination, messageID string) (context.Context, Finisher) {
	return start(ctx, p, system, destination, messageID, observability.SpanKindProducer, observability.OperationPublish)
}

// Consumer starts a consumer span for a message received from destination. It
// returns the span context and a Finisher ending the span. A nil provider, a
// nil tracer, or a nil span yields ctx unchanged and a no-op Finisher.
func Consumer(ctx context.Context, p observability.Provider, system, destination, messageID string) (context.Context, Finisher) {
	return start(ctx, p, system, destination, messageID, observability.SpanKindConsumer, observability.OperationProcess)
}

// start opens one messaging span and returns its Finisher. Telemetry is
// best-effort throughout: every unusable input falls back to ctx and a no-op
// Finisher rather than failing the publish or delivery around it.
func start(
	ctx context.Context,
	p observability.Provider,
	system, destination, messageID string,
	kind observability.SpanKind,
	operation string,
) (context.Context, Finisher) {
	if p == nil {
		return ctx, noopFinisher
	}

	tracer := p.Tracer(ScopeName)
	if tracer == nil {
		return ctx, noopFinisher
	}

	// Resolve the attributes before starting the span so
	// messaging.message.conversation_id reports the caller trace, not the span
	// this call is about to create.
	attrs := messageAttrs(ctx, system, destination, messageID, operation)

	spanCtx, span := observability.StartSpan(ctx, tracer, spanName(system, operation, destination),
		observability.WithSpanKind(kind),
		observability.WithAttributes(attrs...),
	)

	if span == nil {
		return ctx, noopFinisher
	}

	// observability.StartSpan drops its options for a Tracer that does not
	// implement the optional SpanStarter, which would leave the span bare.
	// Setting the same attributes on the span keeps them on the fallback path;
	// a SpanStarter tracer records identical keys and values twice, which every
	// backend resolves to the same attribute.
	span.SetAttributes(attrs...)

	return spanCtx, func(outcome string) {
		if outcome != "" {
			span.SetAttributes(observability.String(AttrOutcome, outcome))
		}

		span.End()
	}
}

// noopFinisher is the Finisher returned when no telemetry is wired.
func noopFinisher(string) {}

// spanName returns "<system> <operation> <destination>". The destination is the
// topic or queue; message IDs and payloads stay out of the name so cardinality
// is bounded by the topic set.
func spanName(system, operation, destination string) string {
	return system + " " + operation + " " + destination
}

// messageAttrs returns the messaging semantic conventions for one message:
// messaging.system, messaging.destination.name, and messaging.operation always,
// plus messaging.message.id and messaging.message.conversation_id when the
// caller supplied a message ID and ctx carries a valid trace. Empty values are
// omitted rather than reported as empty attributes.
func messageAttrs(ctx context.Context, system, destination, messageID, operation string) []observability.Attr {
	attrs := make([]observability.Attr, 0, 5)
	attrs = append(attrs,
		observability.String(observability.MessagingSystem, system),
		observability.String(observability.MessagingDestinationName, destination),
		observability.String(observability.MessagingOperation, operation),
	)

	if messageID != "" {
		attrs = append(attrs, observability.String(observability.MessagingMessageID, messageID))
	}

	if traceID := traceprop.TraceID(ctx); traceID != "" {
		attrs = append(attrs, observability.String(observability.MessagingMessageConversationID, traceID))
	}

	return attrs
}
