package job

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/queue"
)

// BenchmarkWorkerEmptyPoll measures the empty-poll wait path that runLoop
// hits on every idle iteration. It exercises the first empty poll in a run
// (attempt 0, wait 0) and reuses one timer across iterations, matching
// runLoop's own timer reuse, so the per-poll timer allocation is what shows
// up in allocs/op rather than benchmark bookkeeping.
func BenchmarkWorkerEmptyPoll(b *testing.B) {
	w := &Worker{}
	var wg sync.WaitGroup

	timer := time.NewTimer(0)
	defer timer.Stop()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		attempt := 0
		if w.handleEmpty(b.Context(), &wg, &attempt, DefaultMaxPollWait, timer) {
			b.Fatal("handleEmpty returned true for background context")
		}
	}
}

// BenchmarkWorkerProcessNilHeaders measures process on a headerless message
// (unknown-job dead-letter path). The run must not allocate a headers map
// it never writes.
func BenchmarkWorkerProcessNilHeaders(b *testing.B) {
	Reset()
	if err := Register("bench-nilheaders", func(context.Context, string) error { return nil }); err != nil {
		b.Fatalf("Register: %v", err)
	}

	q := &workerStubQueue{ackFn: func(context.Context, queue.Message) error { return nil }}
	w := &Worker{Q: q, Logger: noop.New()}
	msg := queue.Message{Payload: queue.Payload([]byte(`"hi"`)), Attempt: 1}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = w.process(ctx, msg, "low")
	}
}
