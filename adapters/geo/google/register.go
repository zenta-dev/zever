package google

import (
	"github.com/zenta-dev/zever/core/geo"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = geo.Register(geo.Google, New)
}
