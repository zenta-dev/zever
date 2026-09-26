package firebase

import (
	"github.com/zenta-dev/zever/core/flag"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = flag.Register(flag.Firebase, New)
}
