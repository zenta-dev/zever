package redis

import (
	"github.com/zenta-dev/zever/core/lock"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = lock.Register(lock.Redis, New)
}
