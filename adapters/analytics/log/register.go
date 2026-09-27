package log

import (
	"github.com/zenta-dev/zever/core/analytics"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = analytics.Register(analytics.Log, New)
}
