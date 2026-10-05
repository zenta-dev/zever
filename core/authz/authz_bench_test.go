package authz_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
)

// BenchmarkAuthorizeFastPath measures the no-auth no-check short-circuit.
func BenchmarkAuthorizeFastPath(b *testing.B) {
	a := &fakeAuth{}
	p := &fakeChecker{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := authz.Authorize(b.Context(), a, p, authz.Policy{}, "", ""); err != nil {
			b.Fatalf("Authorize() err = %v", err)
		}
	}
}

// BenchmarkAuthorizeFull measures the verify-plus-check hot path.
func BenchmarkAuthorizeFull(b *testing.B) {
	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u1"}}
	p := &fakeChecker{allow: true}
	pol := authz.Policy{AuthRequired: true, Roles: []string{"admin"}, PermissionCheck: "doc.read", ResourceType: "Doc"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := authz.Authorize(b.Context(), a, p, pol, "tok", "r1"); err != nil {
			b.Fatalf("Authorize() err = %v", err)
		}
	}
}

// BenchmarkBearerToken measures bearer token extraction from a header.
func BenchmarkBearerToken(b *testing.B) {
	r := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer abc")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if got := authz.BearerToken(r); got != "abc" {
			b.Fatalf("BearerToken() = %q, want abc", got)
		}
	}
}

// BenchmarkBearerTokenFromMD measures bearer token extraction from metadata.
func BenchmarkBearerTokenFromMD(b *testing.B) {
	ctx := metadata.NewIncomingContext(b.Context(), metadata.Pairs("authorization", "Bearer abc"))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if got := authz.BearerTokenFromMD(ctx); got != "abc" {
			b.Fatalf("BearerTokenFromMD() = %q, want abc", got)
		}
	}
}

// BenchmarkClaimsFromContext measures the claims context round-trip hot path.
func BenchmarkClaimsFromContext(b *testing.B) {
	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u1"}}
	p := &fakeChecker{allow: true}
	var ctx context.Context
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	})
	h := authz.Middleware(a, p, authz.Policy{AuthRequired: true}, nil)(next)
	r := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer tok")
	h.ServeHTTP(httptest.NewRecorder(), r)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, ok := authz.ClaimsFromContext(ctx); !ok {
			b.Fatal("ClaimsFromContext() ok = false, want true")
		}
	}
}

// BenchmarkMiddleware measures the full middleware hot path including the
// error-body write on denial.
func BenchmarkMiddleware(b *testing.B) {
	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u1"}}
	p := &fakeChecker{allow: true}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := authz.Middleware(a, p, authz.Policy{AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"}, nil)(next)
	r := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		h.ServeHTTP(w, r)
	}
}

// BenchmarkUnaryServerInterceptor measures the interceptor hot path.
func BenchmarkUnaryServerInterceptor(b *testing.B) {
	a := &fakeAuth{wantToken: "tok", claims: auth.Claims{Subject: "u1"}}
	p := &fakeChecker{allow: true}
	policies := map[string]authz.Policy{
		"/test.Test/Echo": {AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"},
	}
	iv := authz.UnaryServerInterceptor(a, p, policies)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Test/Echo"}
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	ctx := metadata.NewIncomingContext(b.Context(), metadata.Pairs("authorization", "Bearer tok"))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := iv(ctx, struct{}{}, info, handler); err != nil {
			b.Fatalf("interceptor err = %v", err)
		}
	}
}

// BenchmarkUnaryServerInterceptorDenied measures the interceptor denial path
// including the gRPC status error mapping.
func BenchmarkUnaryServerInterceptorDenied(b *testing.B) {
	a := &fakeAuth{wantToken: "tok"}
	p := &fakeChecker{allow: false}
	policies := map[string]authz.Policy{
		"/test.Test/Echo": {AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"},
	}
	iv := authz.UnaryServerInterceptor(a, p, policies)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Test/Echo"}
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	ctx := metadata.NewIncomingContext(b.Context(), metadata.Pairs("authorization", "Bearer bad"))

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := iv(ctx, struct{}{}, info, handler); err == nil {
			b.Fatal("expected denial error")
		}
	}
}
