package redis

import (
	"github.com/zenta-dev/zever/core/queue"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = queue.Register(queue.Redis, New)
}
