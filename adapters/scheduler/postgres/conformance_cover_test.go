package postgres

import (
	"testing"

	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/scheduler/schedulertest"
)

// TestConformancePostgres proves the leased scheduler passes the scheduler
// kit against a file-backed sqlite database.
func TestConformancePostgres(t *testing.T) {
	t.Parallel()

	schedulertest.Conformance(t, func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler {
		t.Helper()

		s := mustNew(t, Options{Options: scheduler.Options{Dispatcher: withQueue(t, d)}})

		t.Cleanup(func() { _ = s.Close() })

		return s
	})
}

// withQueue returns d when it already carries a queue, else a dispatcher on
// a fresh in-memory queue. The kit always passes a dispatcher, so this is
// belt and braces for direct callers.
func withQueue(t *testing.T, d *job.Dispatcher) *job.Dispatcher {
	t.Helper()

	if d != nil && d.Q != nil {
		return d
	}

	q, err := queuememory.New(queue.Options{})
	if err != nil {
		t.Fatalf("queuememory.New() error = %v", err)
	}

	t.Cleanup(func() { _ = q.Close() })

	return &job.Dispatcher{Q: q}
}
