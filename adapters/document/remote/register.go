package remote

import (
	"github.com/zenta-dev/zever/core/document"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = document.Register(document.Remote, New)
}
