package serve

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
)

// TestWorkerStartsSchedulerAndCloses proves Worker resolves the queue and
// scheduler, starts ticks, and closes cleanly on cancel.
func TestWorkerStartsSchedulerAndCloses(t *testing.T) {
	t.Setenv("ZEVER_ENV", "dev")

	sched := &stubScheduler{}
	q := &stubQueue{}
	started := make(chan struct{})
	sched.onStart = func() { close(started) }

	providers := append(wireStubProviders(t, nil, stubLogger{}),
		container.Provide(queue.DefaultRef(), func(queue.Options) (queue.Queue, error) {
			return q, nil
		}),
		container.Provide(scheduler.DefaultRef(), func(scheduler.Options) (scheduler.Scheduler, error) {
			return sched, nil
		}),
	)

	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)

	go func() {
		errCh <- runWorker(ctx, providers, WithEnvOverride(config.Env{}))
	}()

	deadline, stop := context.WithTimeout(t.Context(), 10*time.Second)
	defer stop()

	select {
	case <-started:
	case <-deadline.Done():
		t.Fatal("scheduler never started")
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runWorker: %v", err)
		}
	case <-deadline.Done():
		t.Fatal("timed out waiting for worker shutdown")
	}

	if !q.closed.Load() {
		t.Fatal("queue not closed on shutdown")
	}
}
