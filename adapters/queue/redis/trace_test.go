package redis

import (
	"testing"

	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/shared/traceprop"
)

// TestDecodeMessage_TracePropagation verifies trace headers survive the
// wire encode/decode round trip: extracting from the decoded message yields
// the producer trace ID. It exercises toWireMessage/decodeMessage directly
// (no live Redis) so it stays deterministic.
func TestDecodeMessage_TracePropagation(t *testing.T) {
	t.Parallel()

	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}

	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}

	pushCtx := trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	headers := queue.Headers(traceprop.Inject(pushCtx, map[string]string{"a": "b"}))
	wire := toWireMessage(queue.NewMessage("jobs", queue.Payload([]byte("hi")), headers))

	raw, marshalErr := jsonMarshal(wire)
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}

	msg, err := decodeMessage(string(raw), "jobs")
	if err != nil {
		t.Fatalf("decodeMessage: %v", err)
	}

	back := trace.SpanContextFromContext(traceprop.ExtractOrBackground(msg.Headers))
	if back.TraceID() != traceID {
		t.Errorf("TraceID = %s, want %s", back.TraceID(), traceID)
	}

	if msg.Headers["a"] != "b" {
		t.Errorf("Headers[a] = %q, want %q", msg.Headers["a"], "b")
	}
}
