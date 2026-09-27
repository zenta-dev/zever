package argon2

import (
	"github.com/zenta-dev/zever/core/password"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = password.Register(password.AdapterArgon2ID, New)
}
