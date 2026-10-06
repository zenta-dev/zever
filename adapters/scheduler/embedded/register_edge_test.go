package embedded

import (
	"testing"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/scheduler"
)

// TestRegister proves Register wires the embedded adapter into the scheduler
// registry, so Open resolves it to a live scheduler.
func TestRegister(t *testing.T) {
	t.Parallel()

	Register()

	s, err := scheduler.Open(scheduler.Embedded, scheduler.Options{Dispatcher: &job.Dispatcher{Q: &stubQueue{}}})
	if err != nil {
		t.Fatalf("Open(%s) after Register: %v", scheduler.Embedded, err)
	}

	if got := s.Name(); got != "embedded" {
		t.Fatalf("Name() = %q, want embedded", got)
	}
}
