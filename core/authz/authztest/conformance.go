// Package authztest provides the conformance kit proving core authorization
// semantics: allow/deny/anon matrix, nil-backend fail-closed behavior, and
// the unknown-method default-allow contract.
package authztest

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/permission"
)

// stubAuth is a fixed-token auth.Auth. Verify succeeds only for wantToken.
type stubAuth struct {
	wantToken string
	claims    auth.Claims
	called    bool
}

func (s *stubAuth) Issue(context.Context, string, map[string]any, time.Duration) (auth.Token, error) {
	return auth.Token{}, nil
}

func (s *stubAuth) Verify(_ context.Context, token string) (auth.Claims, error) {
	s.called = true
	if token == s.wantToken {
		return s.claims, nil
	}
	return auth.Claims{}, auth.ErrInvalidToken
}

func (s *stubAuth) Revoke(context.Context, string) error { return nil }

func (s *stubAuth) Close() error { return nil }

// stubChecker is a fixed-verdict permission.Checker recording its inputs.
type stubChecker struct {
	allow       bool
	err         error
	called      bool
	gotSubject  permission.Subject
	gotAction   string
	gotResource permission.Resource
}

func (s *stubChecker) Can(_ context.Context, subject permission.Subject, action string, resource permission.Resource) (permission.Decision, error) {
	s.called = true
	s.gotSubject = subject
	s.gotAction = action
	s.gotResource = resource
	if s.err != nil {
		return permission.Decision{}, s.err
	}
	return permission.Decision{Allowed: s.allow}, nil
}

// Conformance verifies authz.Authorize semantics with stub backends: the
// allow/deny/anon matrix, nil-Auth fail-closed as Unauthenticated, nil
// Checker fail-closed as PermissionDenied, and the UnaryServerInterceptor
// unknown-method default-allow contract. Tests are deterministic and touch
// no network.
func Conformance(t *testing.T) {
	t.Helper()

	t.Run("Allow", func(t *testing.T) { conformanceAllow(t) })
	t.Run("Deny", func(t *testing.T) { conformanceDeny(t) })
	t.Run("CheckerError", func(t *testing.T) { conformanceCheckerError(t) })
	t.Run("InvalidToken", func(t *testing.T) { conformanceInvalidToken(t) })
	t.Run("MissingToken", func(t *testing.T) { conformanceMissingToken(t) })
	t.Run("Anon", func(t *testing.T) { conformanceAnon(t) })
	t.Run("FastPath", func(t *testing.T) { conformanceFastPath(t) })
	t.Run("NilAuth", func(t *testing.T) { conformanceNilAuth(t) })
	t.Run("NilChecker", func(t *testing.T) { conformanceNilChecker(t) })
	t.Run("UnknownMethodDefaultAllow", func(t *testing.T) { conformanceUnknownMethod(t) })
}

const (
	kitToken  = "kit-token"
	kitAction = "doc.read"
)

func kitAuth() *stubAuth {
	return &stubAuth{wantToken: kitToken, claims: auth.Claims{Subject: "alice"}}
}

func kitPolicy() authz.Policy {
	return authz.Policy{AuthRequired: true, Roles: []string{"admin"}, PermissionCheck: kitAction, ResourceType: "Doc"}
}

func conformanceAllow(t *testing.T) {
	t.Helper()

	a := kitAuth()
	p := &stubChecker{allow: true}
	claims, err := authz.Authorize(t.Context(), a, p, kitPolicy(), kitToken, "r1")
	if err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	// Verified claims flow through: Authorize returns what Auth verified.
	if claims.Subject != "alice" {
		t.Errorf("Authorize() Subject = %q, want alice", claims.Subject)
	}
	if !p.called {
		t.Error("Authorize() did not call checker for a gated policy")
	}
	if p.gotAction != kitAction || p.gotResource.ID != "r1" || p.gotResource.Type != "Doc" {
		t.Errorf("Authorize() checker got action=%q resource=%+v, want doc.read {Doc r1}", p.gotAction, p.gotResource)
	}
}

func conformanceDeny(t *testing.T) {
	t.Helper()

	p := &stubChecker{allow: false}
	_, err := authz.Authorize(t.Context(), kitAuth(), p, kitPolicy(), kitToken, "r1")
	// Checker denials surface as PermissionDenied, never raw: Authorize
	// maps Decision{Allowed:false} to *PermissionDeniedError.
	var denied authz.PermissionDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("Authorize() err = %T %v, want *PermissionDeniedError", err, err)
	}
	if !errors.Is(err, authz.ErrPermissionDenied) {
		t.Fatalf("Authorize() err does not match ErrPermissionDenied: %v", err)
	}
}

func conformanceCheckerError(t *testing.T) {
	t.Helper()

	// Checker infrastructure failures fail closed as PermissionDenied:
	// Authorize never propagates backend errors to callers.
	p := &stubChecker{err: errors.New("kit: backend boom")}
	_, err := authz.Authorize(t.Context(), kitAuth(), p, kitPolicy(), kitToken, "r1")
	var denied authz.PermissionDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("Authorize() err = %T %v, want *PermissionDeniedError", err, err)
	}
}

func conformanceInvalidToken(t *testing.T) {
	t.Helper()

	a := kitAuth()
	p := &stubChecker{allow: true}
	// Auth verification failures surface as Unauthenticated: Authorize
	// wraps any Verify error in *UnauthenticatedError.
	_, err := authz.Authorize(t.Context(), a, p, kitPolicy(), "wrong-token", "r1")
	var unauthenticated authz.UnauthenticatedError
	if !errors.As(err, &unauthenticated) {
		t.Fatalf("Authorize() err = %T %v, want *UnauthenticatedError", err, err)
	}
	if !errors.Is(err, authz.ErrUnauthenticated) {
		t.Fatalf("Authorize() err does not match ErrUnauthenticated: %v", err)
	}
	if p.called {
		t.Error("Authorize() called checker after failed auth")
	}
}

func conformanceMissingToken(t *testing.T) {
	t.Helper()

	a := kitAuth()
	p := &stubChecker{allow: true}
	// A missing bearer token is Unauthenticated without touching Verify:
	// Authorize short-circuits before calling Auth.
	_, err := authz.Authorize(t.Context(), a, p, kitPolicy(), "", "r1")
	var unauthenticated authz.UnauthenticatedError
	if !errors.As(err, &unauthenticated) {
		t.Fatalf("Authorize() err = %T %v, want *UnauthenticatedError", err, err)
	}
	if a.called {
		t.Error("Authorize() called Verify for a missing token")
	}
}

func conformanceAnon(t *testing.T) {
	t.Helper()

	a := kitAuth()
	p := &stubChecker{allow: true}
	pol := authz.Policy{AuthRequired: false, Roles: []string{"admin"}, PermissionCheck: kitAction, ResourceType: "Doc"}
	// Anonymous checks still reach the checker with an empty identity:
	// AuthRequired:false skips Verify but never skips a configured
	// PermissionCheck.
	if _, err := authz.Authorize(t.Context(), a, p, pol, "", "r1"); err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if a.called {
		t.Error("Authorize() called Verify for an anonymous policy")
	}
	if !p.called {
		t.Fatal("Authorize() did not call checker for an anonymous gated policy")
	}
	// Policy.Roles are never asserted for an unverified caller: only
	// AuthRequired policies propagate Roles into the subject.
	if p.gotSubject.ID != "" || len(p.gotSubject.Roles) != 0 {
		t.Errorf("Authorize() anon subject = %+v, want empty ID and roles", p.gotSubject)
	}

	deny := &stubChecker{allow: false}
	if _, err := authz.Authorize(t.Context(), a, deny, pol, "", "r1"); !errors.Is(err, authz.ErrPermissionDenied) {
		t.Errorf("Authorize() anon deny err = %v, want ErrPermissionDenied", err)
	}
}

func conformanceFastPath(t *testing.T) {
	t.Helper()

	a := kitAuth()
	p := &stubChecker{allow: false}
	// A policy with neither auth nor check returns zero claims without
	// touching either backend.
	claims, err := authz.Authorize(t.Context(), a, p, authz.Policy{}, "", "")
	if err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if claims.Subject != "" {
		t.Errorf("Authorize() Subject = %q, want empty", claims.Subject)
	}
	if a.called || p.called {
		t.Errorf("Authorize() touched backends on fast path: auth=%v checker=%v", a.called, p.called)
	}
}

func conformanceNilAuth(t *testing.T) {
	t.Helper()

	// Nil Auth with AuthRequired denies as Unauthenticated instead of
	// panicking: the nil check precedes Verify.
	p := &stubChecker{allow: true}
	_, err := authz.Authorize(t.Context(), nil, p, kitPolicy(), kitToken, "r1")
	var unauthenticated authz.UnauthenticatedError
	if !errors.As(err, &unauthenticated) {
		t.Fatalf("Authorize() err = %T %v, want *UnauthenticatedError", err, err)
	}
	if !errors.Is(err, authz.ErrUnauthenticated) {
		t.Fatalf("Authorize() err does not match ErrUnauthenticated: %v", err)
	}
	if p.called {
		t.Error("Authorize() called checker after nil-auth denial")
	}
}

func conformanceNilChecker(t *testing.T) {
	t.Helper()

	// Nil Checker with a PermissionCheck denies as PermissionDenied
	// instead of panicking: the nil check precedes Can.
	_, err := authz.Authorize(t.Context(), kitAuth(), nil, kitPolicy(), kitToken, "r1")
	var denied authz.PermissionDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("Authorize() err = %T %v, want *PermissionDeniedError", err, err)
	}
	if !errors.Is(err, authz.ErrPermissionDenied) {
		t.Fatalf("Authorize() err does not match ErrPermissionDenied: %v", err)
	}
}

func conformanceUnknownMethod(t *testing.T) {
	t.Helper()

	a := kitAuth()
	p := &stubChecker{allow: false}
	// CURRENT contract, do not change: UnaryServerInterceptor passes
	// requests with no configured policy straight to the handler
	// (default-allow). A missing policy entry is not a denial; closing
	// that gap is a behavior change needing maintainer review.
	iv := authz.UnaryServerInterceptor(a, p, map[string]authz.Policy{})
	called := false
	handler := func(context.Context, any) (any, error) {
		called = true
		return struct{}{}, nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: "/kit.Test/Unknown"}
	if _, err := iv(t.Context(), struct{}{}, info, handler); err != nil {
		t.Fatalf("Invoke() err = %v, want nil (unknown method passes through)", err)
	}
	if !called {
		t.Error("Invoke() did not call handler for an unknown method")
	}
	if a.called || p.called {
		t.Errorf("Invoke() touched backends for unknown method: auth=%v checker=%v", a.called, p.called)
	}
}
