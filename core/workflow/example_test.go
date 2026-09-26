package workflow_test

import (
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
	"github.com/zenta-dev/zever/core/workflow"
)

// ExampleOpen opens the in-memory workflow backend with defaults.
func ExampleOpen() {
	_ = workflow.Register(workflow.Memory, workflowmemory.New)

	w, err := workflow.Open(workflow.Memory, workflow.Options{})
	if err != nil {
		return
	}

	defer func() { _ = w.Close() }()
}
