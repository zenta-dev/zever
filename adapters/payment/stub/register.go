package stub

import (
	"github.com/zenta-dev/zever/core/payment"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = payment.Register(payment.Stub, New)
}
