package embedded

import (
	"testing"

	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/scheduler/schedulertest"
)

// TestConformanceEmbedded proves the embedded scheduler passes the
// scheduler kit over the in-memory queue.
func TestConformanceEmbedded(t *testing.T) {
	t.Parallel()

	schedulertest.Conformance(t, func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler {
		t.Helper()

		if d.Q == nil {
			q, err := queuememory.New(queue.Options{})
			if err != nil {
				t.Fatalf("queuememory.New() error = %v", err)
			}

			t.Cleanup(func() { _ = q.Close() })

			d = &job.Dispatcher{Q: q}
		}

		s, err := New(scheduler.Options{Dispatcher: d})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return s
	})
}
