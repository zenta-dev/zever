package permission_test

import (
	"context"

	"github.com/zenta-dev/zever/permission"
	"github.com/zenta-dev/zever/permission/noop"
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
