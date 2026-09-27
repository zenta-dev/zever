package local

import (
	"github.com/zenta-dev/zever/core/crypto"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = crypto.Register(crypto.AdapterLocal, New)
}
