package redis

import (
	"github.com/zenta-dev/zever/core/eventbus"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = eventbus.Register(eventbus.Redis, New)
}
