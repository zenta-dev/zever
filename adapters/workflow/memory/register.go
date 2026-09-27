package memory

import (
	"github.com/zenta-dev/zever/core/workflow"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = workflow.Register(workflow.Memory, New)
}
