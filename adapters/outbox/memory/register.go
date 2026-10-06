package memory

import (
	"github.com/zenta-dev/zever/core/outbox"
)

// Register wires this adapter into its battery registry. Call from your app's
// main or generated app.go; no init magic. The core Options carry no
// Publisher, so a store opened this way records nothing until a Publisher is
// wired through New directly.
func Register() {
	_ = outbox.Register(outbox.Memory, func(o outbox.Options) (outbox.Store, error) {
		return New(Options{Options: o})
	})
}
