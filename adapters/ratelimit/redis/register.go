package redis

import (
	"github.com/zenta-dev/zever/core/ratelimit"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = ratelimit.Register(ratelimit.Redis, New)
}
