package stdout

import (
	"github.com/zenta-dev/zever/core/observability"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = observability.Register(observability.Stdout, New)
}
