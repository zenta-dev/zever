package workflow_test

import (
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
	"github.com/zenta-dev/zever/core/workflow"
)

// Example opens the in-memory workflow backend with defaults.
func Example() {
	w, err := workflowmemory.New(workflow.Options{})
	if err != nil {
		return
	}

	defer func() { _ = w.Close() }()
}
