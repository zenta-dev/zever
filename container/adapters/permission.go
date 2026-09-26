package adapters

import (
	"github.com/zenta-dev/zever/permission"
	permissioncasbin "github.com/zenta-dev/zever/permission/casbin"
)

// RegisterPermission registers the policy-engine permission adapter the
// core container no longer imports: permission/casbin. The noop and rbac
// adapters stay wired by the container. Registration only fills factory
// maps; it performs no I/O. Duplicate registrations are ignored, so
// calling RegisterPermission more than once (or alongside RegisterAll) is
// safe.
func RegisterPermission() {
	_ = permission.Register(permission.Casbin, permissioncasbin.New)
}
