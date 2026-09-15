package authz_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/authz"
)

func TestSentinels(t *testing.T) {
	t.Parallel()

	if authz.ErrUnauthenticated == nil || authz.ErrUnauthenticated.Error() != "authz: unauthenticated" {
		t.Fatalf("ErrUnauthenticated = %v, want 'authz: unauthenticated'", authz.ErrUnauthenticated)
	}
	if authz.ErrPermissionDenied == nil || authz.ErrPermissionDenied.Error() != "authz: permission denied" {
		t.Fatalf("ErrPermissionDenied = %v, want 'authz: permission denied'", authz.ErrPermissionDenied)
	}
}

func TestUnauthenticatedErrorUnwrap(t *testing.T) {
	t.Parallel()

	err := &authz.UnauthenticatedError{Reason: "missing bearer token"}
	if !errors.Is(err, authz.ErrUnauthenticated) {
		t.Fatalf("errors.Is(%v, ErrUnauthenticated) = false", err)
	}
	var target *authz.UnauthenticatedError
	if !errors.As(err, &target) {
		t.Fatal("errors.As for *UnauthenticatedError failed")
	}
	if strings.Contains(err.Error(), "tokensecret") {
		t.Fatal("error must not echo tokens")
	}
}

func TestPermissionDeniedErrorUnwrap(t *testing.T) {
	t.Parallel()

	err := &authz.PermissionDeniedError{Reason: "permission denied"}
	if !errors.Is(err, authz.ErrPermissionDenied) {
		t.Fatalf("errors.Is(%v, ErrPermissionDenied) = false", err)
	}
	var target *authz.PermissionDeniedError
	if !errors.As(err, &target) {
		t.Fatal("errors.As for *PermissionDeniedError failed")
	}
	if errors.Is(err, authz.ErrUnauthenticated) {
		t.Fatal("permission denied must not match ErrUnauthenticated")
	}
}
