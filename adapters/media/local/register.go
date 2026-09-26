package local

import (
	"github.com/zenta-dev/zever/core/media"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = media.Register(media.Local, New)
}
