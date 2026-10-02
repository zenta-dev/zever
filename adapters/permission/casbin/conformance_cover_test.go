package casbin_test

import (
	"testing"

	"github.com/zenta-dev/zever/adapters/permission/casbin"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/permission/permissiontest"
)

// newConformanceChecker builds a checker with the canonical kit policy
// (allow admin doc.read, deny admin doc.delete) plus the Roles allowlist
// gating alice to admin: without the allowlist entry, alice's asserted
// admin role is skipped as a privilege-escalation guard and the allow
// case would deny.
func newConformanceChecker(t *testing.T) permission.Checker {
	t.Helper()

	c, err := casbin.New(permission.Options{
		Rules: []permission.Rule{
			{Role: "admin", Action: "doc.read"},
			{Role: "admin", Action: "doc.delete", Effect: permission.Deny},
		},
		Roles: map[string][]string{"alice": {"admin"}},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return c
}

// TestCasbinConformance proves the Casbin checker honors the permission
// contract via the shared conformance kit (no network).
func TestCasbinConformance(t *testing.T) {
	t.Parallel()

	permissiontest.Conformance(t, func(t *testing.T) permission.Checker {
		t.Helper()
		return newConformanceChecker(t)
	})
}

// TestCasbinConformanceCancelledCtx proves fail-closed behavior under a
// cancelled context: Can returns an error without touching the enforcer,
// matching the nil-ctx-as-Background contract for the non-cancelled path.
func TestCasbinConformanceCancelledCtx(t *testing.T) {
	t.Parallel()

	permissiontest.ConformanceCancelledCtx(t, func(t *testing.T) permission.Checker {
		t.Helper()
		return newConformanceChecker(t)
	})
}
