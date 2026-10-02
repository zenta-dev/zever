package schedulertest_test

import (
	"testing"

	"github.com/zenta-dev/zever/adapters/scheduler/embedded"
	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/scheduler/schedulertest"
)

// TestConformanceEmbedded proves the kit passes against the embedded adapter.
func TestConformanceEmbedded(t *testing.T) {
	t.Parallel()

	schedulertest.Conformance(t, func(t *testing.T, d *job.Dispatcher) scheduler.Scheduler {
		t.Helper()

		s, err := embedded.New(scheduler.Options{Dispatcher: d})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return s
	})
}
