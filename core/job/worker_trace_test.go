package job

import (
	"context"
	"encoding/json/v2"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/queue"
)

// TestWorkerLaunchHandler_ExtractsTrace verifies the consume-side gap is
// closed: a job message carrying a producer traceparent gives the handler a
// context with the same trace ID.
func TestWorkerLaunchHandler_ExtractsTrace(t *testing.T) {
	Reset()

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
