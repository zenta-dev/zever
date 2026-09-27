package rbac

import (
	"github.com/zenta-dev/zever/core/permission"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = permission.Register(permission.RBAC, New)
}
