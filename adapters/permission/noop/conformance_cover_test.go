package noop_test

import (
	"testing"

	"github.com/zenta-dev/zever/adapters/permission/noop"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/permission/permissiontest"
)

// TestNoopConformance proves the noop checker honors the deny-all contract
// via the shared conformance kit (no network). It runs the deny-all kit
// only: noop has no allow path, so the allow-parity kit cannot apply.
func TestNoopConformance(t *testing.T) {
	t.Parallel()

	permissiontest.ConformanceDenyAll(t, func(t *testing.T) permission.Checker {
		t.Helper()

		c, err := noop.New(permission.Options{})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return c
	})
}
