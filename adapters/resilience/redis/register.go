package redis

import (
	"github.com/zenta-dev/zever/core/resilience"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = resilience.Register(resilience.Redis, func(opts resilience.Options) (resilience.Manager, error) {
		return New(Options{Options: opts})
	})
}
