package memory_test

import (
	"testing"

	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/core/workflow/workflowtest"
)

// TestConformanceMemory proves the memory engine passes the workflow kit.
func TestConformanceMemory(t *testing.T) {
	t.Parallel()

	workflowtest.Conformance(t, func(t *testing.T) workflow.Workflow {
		t.Helper()

		w, err := workflowmemory.New(workflow.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = w.Close() })

		return w
	})
}
