package rbac_test

import (
	"testing"

	"github.com/zenta-dev/zever/adapters/permission/rbac"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/permission/permissiontest"
)

// conformanceRules is the canonical kit policy: allow admin doc.read,
// deny admin doc.delete, deny everything else by default.
func conformanceRules() []permission.Rule {
	return []permission.Rule{
		{Role: "admin", Action: "doc.read"},
		{Role: "admin", Action: "doc.delete", Effect: permission.Deny},
	}
}

// TestRBACConformance proves the RBAC checker honors the permission
// contract via the shared conformance kit (no network). It runs the
// allow/deny/default-deny/nil-ctx kit only: the rbac adapter is
// ctx-insensitive (Can never returns an error and ignores cancellation),
// so ConformanceCancelledCtx does not apply -- that exemption is current
// behavior, not a gap, and is documented here rather than changed.
func TestRBACConformance(t *testing.T) {
	t.Parallel()

	permissiontest.Conformance(t, func(t *testing.T) permission.Checker {
		t.Helper()

		c, err := rbac.New(permission.Options{Rules: conformanceRules()})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		return c
	})
}
