package single

import (
	"github.com/zenta-dev/zever/core/tenant"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = tenant.Register(tenant.Single, New)
}
