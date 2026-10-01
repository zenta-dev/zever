package memory

import (
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/shared/traceprop"
)

// TestMemory_TracePropagation_RoundTrip verifies a producer span injected on
// Push survives Pop: extracting from the popped headers yields the same trace
// ID the producer started with.
func TestMemory_TracePropagation_RoundTrip(t *testing.T) {
	t.Parallel()

	q := newQueue(t, queue.Options{Buffer: 10, PollTimeout: 20 * time.Millisecond, VisibilityTimeout: time.Second})

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

	if pushErr := q.Push(pushCtx, "jobs", queue.Payload([]byte("hi")), queue.Headers{"a": "b"}); pushErr != nil {
		t.Fatalf("Push: %v", pushErr)
	}

	msg, err := q.Pop(t.Context(), "jobs")
	if err != nil {
		t.Fatalf("Pop: %v", err)
	}

	back := trace.SpanContextFromContext(traceprop.ExtractOrBackground(msg.Headers))
	if back.TraceID() != traceID {
		t.Errorf("TraceID = %s, want %s", back.TraceID(), traceID)
	}

	if msg.Headers["a"] != "b" {
		t.Errorf("Headers[a] = %q, want %q", msg.Headers["a"], "b")
	}
}
