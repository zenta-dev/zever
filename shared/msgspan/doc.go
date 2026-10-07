// Package msgspan starts OpenTelemetry messaging spans at every publish and
// consume boundary (eventbus publish/deliver, queue push, job dispatch drain).
//
// Producer and Consumer return the span context plus a Finisher that ends the
// span and records its outcome. Both are best-effort: a nil
// observability.Provider, a nil tracer, or a nil span makes every call a no-op,
// so telemetry never fails a publish or a delivery.
//
// Spans carry the messaging semantic conventions -- messaging.system,
// messaging.destination.name, messaging.operation, messaging.message.id, and
// messaging.message.conversation_id (the trace ID) -- with SpanKindProducer on
// publish and SpanKindConsumer on receive, so off-the-shelf OTel queries and
// dashboards line up. The span name is "<system> <operation> <destination>":
// message IDs and payloads never reach a span name, keeping cardinality bounded
// by the topic set.
package msgspan
