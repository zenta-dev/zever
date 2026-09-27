package redis

import (
	"github.com/zenta-dev/zever/core/session"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = session.Register(session.Redis, New)
}
