package memory

import (
	"github.com/zenta-dev/zever/core/idempotency"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = idempotency.Register(idempotency.Memory, New)
}
