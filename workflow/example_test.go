package workflow_test

import (
	"github.com/zenta-dev/zever/workflow"
	workflowmemory "github.com/zenta-dev/zever/workflow/memory"
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
