package authz_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/permission"
)

// nilCtx is a nil context used to prove the battery never dereferences ctx
// before the adapter boundary.
var nilCtx context.Context

func TestEdgeAuthorize_nilContextFastPath(t *testing.T) {
	t.Parallel()

	// The fast path returns before any backend or context use.
	claims, err := authz.Authorize(nilCtx, nil, nil, authz.Policy{}, "", "")
	if err != nil {
		t.Fatalf("Authorize(nil ctx) err = %v, want nil", err)
	}
	if claims.Subject != "" {
		t.Fatalf("Authorize(nil ctx) claims = %+v, want zero", claims)
	}
}

func TestEdgeBearerToken_schemeOnlyNoSpace(t *testing.T) {
	t.Parallel()

	// "Bearer" with no trailing space and no token fails closed.
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"exact bearer", "Bearer", ""},
		{"lowercase no space", "bearer", ""},
		{"trailing tab", "Bearer abc\t", "abc"},
		{"tab is not a separator", "Bearer\tabc", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.Header.Set("Authorization", tc.header)
			if got := authz.BearerToken(r); got != tc.want {
				t.Fatalf("BearerToken(%q) = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

func TestEdgeBearerTokenFromMD_emptyValue(t *testing.T) {
	t.Parallel()

	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", ""))
	if got := authz.BearerTokenFromMD(ctx); got != "" {
		t.Fatalf("BearerTokenFromMD(empty) = %q, want empty", got)
	}
}

func TestEdgeMiddleware_nilAuth(t *testing.T) {
	t.Parallel()

	p := &fakeChecker{allow: true}
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	h := authz.Middleware(nil, p, authz.Policy{AuthRequired: true}, nil)(next)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if called {
		t.Fatal("handler ran with nil authenticator")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", w.Code)
	}
	if p.called {
		t.Fatal("checker called with nil authenticator")
	}
}

func TestEdgeMiddleware_nilChecker(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u"}}
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true })
	pol := authz.Policy{AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"}
	h := authz.Middleware(a, nil, pol, nil)(next)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if called {
		t.Fatal("handler ran with nil checker")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", w.Code)
	}
}

func TestEdgeMiddleware_publicPolicy(t *testing.T) {
	t.Parallel()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	h := authz.Middleware(nil, nil, authz.Policy{}, nil)(next)

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !called {
		t.Fatal("handler did not run for public policy")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", w.Code)
	}
}

func TestEdgeGrpcStatusError_mappings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		token string
		code  codes.Code
	}{
		{"unauthenticated", "Bearer bad", codes.Unauthenticated},
		{"denied", "Bearer tok", codes.PermissionDenied},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			iv := authz.UnaryServerInterceptor(&fakeAuth{wantToken: "tok"}, &fakeChecker{allow: false}, map[string]authz.Policy{
				"/test.Test/Echo": {AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"},
			})
			info := &grpc.UnaryServerInfo{FullMethod: "/test.Test/Echo"}
			handler := func(context.Context, any) (any, error) { return "ok", nil }
			ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", tc.token))

			_, err := iv(ctx, struct{}{}, info, handler)
			if status.Code(err) != tc.code {
				t.Fatalf("Code() = %v, want %v (err=%v)", status.Code(err), tc.code, err)
			}
		})
	}
}

func TestEdgeAuthorize_emptyResourceType(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{allow: true}
	pol := authz.Policy{AuthRequired: true, PermissionCheck: "x.y"}
	if _, err := authz.Authorize(t.Context(), a, p, pol, "tok", "r1"); err != nil {
		t.Fatalf("Authorize() err = %v, want nil", err)
	}
	if p.gotResource.Type != "" {
		t.Fatalf("resource Type = %q, want empty", p.gotResource.Type)
	}
	if p.gotResource.ID != "r1" {
		t.Fatalf("resource ID = %q, want r1", p.gotResource.ID)
	}
}

func TestEdgeAuthorize_errorReasonStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		pol      authz.Policy
		token    string
		nilAuth  bool
		nilCheck bool
		reason   string
	}{
		{"missing token", authz.Policy{AuthRequired: true}, "", false, false, "missing bearer token"},
		{"nil auth", authz.Policy{AuthRequired: true}, "tok", true, false, "missing authenticator"},
		{"invalid token", authz.Policy{AuthRequired: true}, "bad", false, false, "invalid or expired token"},
		{"nil checker", authz.Policy{AuthRequired: true, PermissionCheck: "x.y"}, "tok", false, true, "permission check failed"},
		{"denied", authz.Policy{AuthRequired: true, PermissionCheck: "x.y"}, "tok", false, false, "permission denied"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var a auth.Auth
			if !tc.nilAuth {
				a = &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u"}}
			}
			var p permission.Checker
			if !tc.nilCheck {
				p = &fakeChecker{allow: false}
			}
			_, err := authz.Authorize(t.Context(), a, p, tc.pol, tc.token, "")
			if err == nil {
				t.Fatal("Authorize() err = nil, want error")
			}
			if !containsReason(err, tc.reason) {
				t.Fatalf("Authorize() err = %q, want reason %q", err.Error(), tc.reason)
			}
		})
	}
}

func containsReason(err error, reason string) bool {
	var ue authz.UnauthenticatedError
	var pe authz.PermissionDeniedError
	switch {
	case errors.As(err, &ue):
		return ue.Reason == reason
	case errors.As(err, &pe):
		return pe.Reason == reason
	default:
		return false
	}
}
