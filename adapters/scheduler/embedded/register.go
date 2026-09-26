package embedded

import (
	"github.com/zenta-dev/zever/core/scheduler"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = scheduler.Register(scheduler.Embedded, New)
}
