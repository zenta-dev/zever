package job

import (
	"context"
	"encoding/json/v2"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/shared/traceprop"
)

type consumeStubTracer struct {
	trace.Tracer
	mu    sync.Mutex
	names []string
}

func (t *consumeStubTracer) Start(ctx context.Context, name string, _ ...trace.SpanStartOption) (context.Context, trace.Span) {
	t.mu.Lock()
	t.names = append(t.names, name)
	t.mu.Unlock()
	// Preserve the extracted parent TraceID like a real SDK consumer span.
	parent := trace.SpanContextFromContext(ctx)
	var sid trace.SpanID
	sid[0], sid[7] = 0xab, 0x01
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    parent.TraceID(),
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	})
	span := &traceStubSpan{sc: sc}
	return trace.ContextWithSpan(ctx, span), span
}

type consumeStubProvider struct {
	trace.TracerProvider
	tracer *consumeStubTracer
}

func (p *consumeStubProvider) Tracer(_ string, _ ...trace.TracerOption) trace.Tracer {
	return p.tracer
}

// TestWorkerLaunchHandler_ExtractsTrace verifies the consume-side gap is
// closed: a job message carrying a producer traceparent gives the handler a
// context with the same trace ID.
func TestWorkerLaunchHandler_ExtractsTrace(t *testing.T) {
	Reset()

	prev := otel.GetTracerProvider()
	st := &consumeStubTracer{}
	otel.SetTracerProvider(&consumeStubProvider{tracer: st})
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	const traceIDHex = "4bf92f3577b34da6a3ce929d0e0e4736"

	gotCh := make(chan string, 1)

	if err := Register("trace-job", func(ctx context.Context, _ string) error {
		gotCh <- trace.SpanContextFromContext(ctx).TraceID().String()
		return nil
	}); err != nil {
		t.Fatalf("Register trace-job: %v", err)
	}

	payload, _ := json.Marshal("x")
	msg := queue.Message{
		Payload: queue.Payload(payload),
		Headers: queue.Headers{
			headerJobName: "trace-job",
			"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		},
		Attempt: 1,
	}

	q := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w := &Worker{Q: q, DrainTimeout: 500 * time.Millisecond, Logger: noop.New()}

	sem := make(chan struct{}, 1)
	sem <- struct{}{}

	var wg sync.WaitGroup

	w.launchHandler(t.Context(), msg, "low", sem, &wg)

	select {
	case got := <-gotCh:
		if got != traceIDHex {
			t.Errorf("handler TraceID = %s, want %s", got, traceIDHex)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler not called in time")
	}

	st.mu.Lock()
	found := false
	for _, n := range st.names {
		if n == "queue.consume" {
			found = true
		}
	}
	names := append([]string(nil), st.names...)
	st.mu.Unlock()
	if !found {
		t.Errorf("spans = %v, want queue.consume", names)
	}
	_ = traceprop.ScopeName

	done := make(chan struct{})
	go func() {
		w.waitDrain(&wg)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("launchHandler didn't complete")
	}
}
