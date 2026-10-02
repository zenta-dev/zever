package authz_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
)

func TestAuthorize_NilAuthenticatorFailsClosed(t *testing.T) {
	t.Parallel()
	p := &fakeChecker{allow: true}
	_, err := authz.Authorize(t.Context(), nil, p, authz.Policy{AuthRequired: true}, "tok", "")
	if err == nil {
		t.Fatal("Authorize(nil auth) err = nil, want unauthenticated")
	}
	var ue *authz.UnauthenticatedError
	if !errors.As(err, &ue) {
		t.Fatalf("Authorize(nil auth) err = %T %v, want *UnauthenticatedError", err, err)
	}
	if p.called {
		t.Fatal("nil authenticator must not reach the checker")
	}
}

func TestAuthorize_NilCheckerFailsClosed(t *testing.T) {
	t.Parallel()
	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u"}}
	_, err := authz.Authorize(t.Context(), a, nil, authz.Policy{
		AuthRequired:    true,
		PermissionCheck: "doc.read",
		ResourceType:    "Doc",
	}, "tok", "r1")
	if err == nil {
		t.Fatal("Authorize(nil checker) err = nil, want permission denied")
	}
	var pe *authz.PermissionDeniedError
	if !errors.As(err, &pe) {
		t.Fatalf("Authorize(nil checker) err = %T %v, want *PermissionDeniedError", err, err)
	}
}

func TestAuthorize_NilBackendsPublicPolicy(t *testing.T) {
	t.Parallel()
	if _, err := authz.Authorize(t.Context(), nil, nil, authz.Policy{}, "", ""); err != nil {
		t.Fatalf("Authorize(public, nil backends) err = %v, want nil", err)
	}
}

func TestAuthorize_PolicyRolesNotAliased(t *testing.T) {
	t.Parallel()
	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{allow: true}
	pol := authz.Policy{AuthRequired: true, Roles: []string{"admin"}, PermissionCheck: "doc.read"}
	if _, err := authz.Authorize(t.Context(), a, p, pol, "tok", ""); err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	pol.Roles[0] = "MUT"
	if len(p.gotSubject.Roles) != 1 || p.gotSubject.Roles[0] != "admin" {
		t.Fatalf("checker roles = %v, want [admin] (Policy.Roles mutated after call)", p.gotSubject.Roles)
	}
}
