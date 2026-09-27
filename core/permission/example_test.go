package permission_test

import (
	"context"

	"github.com/zenta-dev/zever/adapters/permission/noop"
	"github.com/zenta-dev/zever/core/permission"
)

// ExampleOpen opens the noop checker and runs a check.
func ExampleOpen() {
	_ = permission.Register(permission.Noop, noop.New)

	checker, err := permission.Open(permission.Noop, permission.Options{})
	if err != nil {
		return
	}

	_, _ = checker.Can(
		context.Background(),
		permission.Subject{ID: "u1"},
		"read",
		permission.Resource{Type: "doc", ID: "d1"},
	)
}
