package authz_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/authz"
	"github.com/zenta-dev/zever/permission"
)

type fakeAuth struct {
	wantToken string
	claims    auth.Claims
	err       error
	called    bool
}

func (f *fakeAuth) Issue(context.Context, string, map[string]any, time.Duration) (auth.Token, error) {
	return auth.Token{}, nil
}

func (f *fakeAuth) Verify(_ context.Context, token string) (auth.Claims, error) {
	f.called = true
	if token == f.wantToken {
		return f.claims, nil
	}
	if f.err != nil {
		return auth.Claims{}, f.err
	}
	return auth.Claims{}, auth.ErrInvalidToken
}

func (f *fakeAuth) Revoke(context.Context, string) error { return nil }
func (f *fakeAuth) Close() error                         { return nil }

type fakeChecker struct {
	allow       bool
	err         error
	called      bool
	gotSubject  permission.Subject
	gotAction   string
	gotResource permission.Resource
}

func (f *fakeChecker) Can(_ context.Context, s permission.Subject, action string, r permission.Resource) (permission.Decision, error) {
	f.called = true
	f.gotSubject = s
	f.gotAction = action
	f.gotResource = r
	if f.err != nil {
		return permission.Decision{}, f.err
	}
	return permission.Decision{Allowed: f.allow, Reason: "test"}, nil
}

func TestAuthorizeFastPathNoCalls(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{}
	p := &fakeChecker{}
	claims, err := authz.Authorize(context.Background(), a, p, authz.Policy{}, "", "")
	if err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if claims.Subject != "" {
		t.Fatalf("Authorize() claims = %+v, want zero", claims)
	}
	if a.called || p.called {
		t.Fatalf("fast path called backends: auth=%v checker=%v", a.called, p.called)
	}
}

func TestAuthorizeMissingToken(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "tok"}
	p := &fakeChecker{}
	_, err := authz.Authorize(context.Background(), a, p, authz.Policy{AuthRequired: true}, "", "")
	if err == nil {
		t.Fatal("Authorize() err = nil, want unauthenticated")
	}
	var ue *authz.UnauthenticatedError
	if !errors.As(err, &ue) {
		t.Fatalf("Authorize() err = %T %v, want *UnauthenticatedError", err, err)
	}
	if !errors.Is(err, authz.ErrUnauthenticated) {
		t.Fatalf("Authorize() err does not match ErrUnauthenticated: %v", err)
	}
	if a.called {
		t.Fatal("missing token must not call Verify")
	}
}

func TestAuthorizeInvalidToken(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", err: auth.ErrInvalidToken}
	p := &fakeChecker{}
	_, err := authz.Authorize(context.Background(), a, p, authz.Policy{AuthRequired: true}, "bad", "")
	if err == nil {
		t.Fatal("Authorize() err = nil, want unauthenticated")
	}
	var ue *authz.UnauthenticatedError
	if !errors.As(err, &ue) {
		t.Fatalf("Authorize() err = %T %v, want *UnauthenticatedError", err, err)
	}
	if !errors.Is(err, authz.ErrUnauthenticated) {
		t.Fatalf("Authorize() err does not match ErrUnauthenticated: %v", err)
	}
	if !a.called {
		t.Fatal("invalid token must call Verify")
	}
}

func TestAuthorizeSkipIfNoCheck(t *testing.T) {
	t.Parallel()

	want := auth.Claims{Subject: "u1", Custom: map[string]any{"k": "v"}}
	a := &fakeAuth{wantToken: "tok", claims: want}
	p := &fakeChecker{}
	got, err := authz.Authorize(context.Background(), a, p, authz.Policy{AuthRequired: true}, "tok", "")
	if err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if got.Subject != "u1" {
		t.Fatalf("Authorize() subject = %q, want u1", got.Subject)
	}
	if p.called {
		t.Fatal("empty PermissionCheck must not call checker")
	}
}

func TestAuthorizeSubjectActionResourceMapping(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{
		wantToken: "tok",
		claims: auth.Claims{Subject: "user-1", Custom: map[string]any{
			"team":   "eng",
			"count":  3,
			"flag":   true,
			"nested": map[string]any{"x": "y"},
			"nilv":   nil,
		}},
	}
	p := &fakeChecker{allow: true}
	pol := authz.Policy{AuthRequired: true, Roles: []string{"admin", "dev"}, PermissionCheck: "order.read", ResourceType: "Order"}
	_, err := authz.Authorize(context.Background(), a, p, pol, "tok", "res-9")
	if err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if !p.called {
		t.Fatal("checker not called")
	}
	if p.gotSubject.ID != "user-1" {
		t.Fatalf("subject ID = %q, want user-1", p.gotSubject.ID)
	}
	if len(p.gotSubject.Roles) != 2 || p.gotSubject.Roles[0] != "admin" || p.gotSubject.Roles[1] != "dev" {
		t.Fatalf("roles = %v, want [admin dev]", p.gotSubject.Roles)
	}
	if len(p.gotSubject.Attributes) != 1 || p.gotSubject.Attributes["team"] != "eng" {
		t.Fatalf("attrs = %v, want only string-valued Custom", p.gotSubject.Attributes)
	}
	if p.gotAction != "order.read" {
		t.Fatalf("action = %q, want order.read", p.gotAction)
	}
	if p.gotResource.Type != "Order" || p.gotResource.ID != "res-9" {
		t.Fatalf("resource = %+v, want {Order res-9}", p.gotResource)
	}
}

func TestAuthorizeNonStringCustomDropped(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{
		wantToken: "tok",
		claims:    auth.Claims{Subject: "u", Custom: map[string]any{"n": 3, "b": true}},
	}
	p := &fakeChecker{allow: true}
	pol := authz.Policy{AuthRequired: true, PermissionCheck: "x.y"}
	if _, err := authz.Authorize(context.Background(), a, p, pol, "tok", ""); err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if len(p.gotSubject.Attributes) != 0 {
		t.Fatalf("attrs = %v, want empty (non-strings dropped)", p.gotSubject.Attributes)
	}
}

func TestAuthorizeAnonymousCanEval(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{}
	p := &fakeChecker{allow: true}
	pol := authz.Policy{AuthRequired: false, PermissionCheck: "doc.read", ResourceType: "Doc"}
	_, err := authz.Authorize(context.Background(), a, p, pol, "", "r1")
	if err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if !p.called {
		t.Fatal("anonymous check must still call checker")
	}
	if a.called {
		t.Fatal("AuthRequired=false must not call Verify")
	}
	if p.gotSubject.ID != "" {
		t.Fatalf("anonymous subject ID = %q, want empty", p.gotSubject.ID)
	}
}

// TestAuthorizeAnonymousRolesIgnored guards against a fixed, policy-configured
// Roles list being asserted for an unauthenticated caller: AuthRequired:false
// means the caller's identity was never verified, so it must never be
// evaluated as holding any role, however the Policy is configured. Without
// this, a Policy meant as "public route, still gated" (AuthRequired: false,
// PermissionCheck set, Roles: [...]) would silently grant every anonymous
// caller those roles.
func TestAuthorizeAnonymousRolesIgnored(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{}
	p := &fakeChecker{allow: true}
	pol := authz.Policy{AuthRequired: false, PermissionCheck: "doc.delete", ResourceType: "Doc", Roles: []string{"admin"}}
	_, err := authz.Authorize(context.Background(), a, p, pol, "", "r1")
	if err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if a.called {
		t.Fatal("AuthRequired=false must not call Verify")
	}
	if len(p.gotSubject.Roles) != 0 {
		t.Fatalf("anonymous subject Roles = %v, want empty (Policy.Roles must not be asserted for an unverified caller)", p.gotSubject.Roles)
	}
}

func TestAuthorizeEmptyResourceIDEval(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{allow: true}
	pol := authz.Policy{AuthRequired: true, PermissionCheck: "order.list", ResourceType: "Order"}
	_, err := authz.Authorize(context.Background(), a, p, pol, "tok", "")
	if err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if !p.called {
		t.Fatal("empty resourceID must still call checker (no bypass skip)")
	}
	if p.gotResource.ID != "" {
		t.Fatalf("resource ID = %q, want empty", p.gotResource.ID)
	}
}

func TestAuthorizeCheckerErrMapping(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{err: errors.New("boom")}
	pol := authz.Policy{AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"}
	_, err := authz.Authorize(context.Background(), a, p, pol, "tok", "r")
	if err == nil {
		t.Fatal("Authorize() err = nil, want permission denied")
	}
	var pe *authz.PermissionDeniedError
	if !errors.As(err, &pe) {
		t.Fatalf("Authorize() err = %T %v, want *PermissionDeniedError", err, err)
	}
	if !errors.Is(err, authz.ErrPermissionDenied) {
		t.Fatalf("Authorize() err does not match ErrPermissionDenied: %v", err)
	}
}

func TestAuthorizeDeniedMapping(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{allow: false}
	pol := authz.Policy{AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"}
	_, err := authz.Authorize(context.Background(), a, p, pol, "tok", "r")
	if err == nil {
		t.Fatal("Authorize() err = nil, want permission denied")
	}
	var pe *authz.PermissionDeniedError
	if !errors.As(err, &pe) {
		t.Fatalf("Authorize() err = %T %v, want *PermissionDeniedError", err, err)
	}
	if !errors.Is(err, authz.ErrPermissionDenied) {
		t.Fatalf("Authorize() err does not match ErrPermissionDenied: %v", err)
	}
}
