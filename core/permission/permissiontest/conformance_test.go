package permissiontest_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/permission/permissiontest"
)

// stubChecker is a strict in-memory checker with the canonical policy:
// allow admin doc.read, deny admin doc.delete, deny everything else. It
// observes ctx (nil as Background, cancelled fail-closed) to prove the
// kit passes for a ctx-sensitive backend.
type stubChecker struct{}

func (stubChecker) Can(ctx context.Context, subject permission.Subject, action string, _ permission.Resource) (permission.Decision, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return permission.Decision{}, err
	}
	hasAdmin := false
	for _, r := range subject.Roles {
		if r == "admin" {
			hasAdmin = true
		}
	}
	if hasAdmin && action == "doc.delete" {
		return permission.Decision{Allowed: false, Reason: "explicit_deny"}, nil
	}
	if hasAdmin && action == "doc.read" {
		return permission.Decision{Allowed: true, Reason: "allow"}, nil
	}
	return permission.Decision{Allowed: false, Reason: "implicit_deny"}, nil
}

// stubDeny is an always-deny checker proving the deny-all kit passes.
type stubDeny struct{}

func (stubDeny) Can(context.Context, permission.Subject, string, permission.Resource) (permission.Decision, error) {
	return permission.Decision{Allowed: false, Reason: "implicit_deny"}, nil
}

// TestConformanceStub proves the kit passes against a strict ctx-sensitive backend.
func TestConformanceStub(t *testing.T) {
	t.Parallel()

	permissiontest.Conformance(t, func(t *testing.T) permission.Checker {
		t.Helper()
		return stubChecker{}
	})
}

// TestConformanceCancelledCtxStub proves the fail-closed kit passes for a ctx-sensitive backend.
func TestConformanceCancelledCtxStub(t *testing.T) {
	t.Parallel()

	permissiontest.ConformanceCancelledCtx(t, func(t *testing.T) permission.Checker {
		t.Helper()
		return stubChecker{}
	})
}

// TestConformanceDenyAllStub proves the deny-all kit passes.
func TestConformanceDenyAllStub(t *testing.T) {
	t.Parallel()

	permissiontest.ConformanceDenyAll(t, func(t *testing.T) permission.Checker {
		t.Helper()
		return stubDeny{}
	})
}
