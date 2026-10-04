package authz_test

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/permission"
)

type benchAuth struct{}

func (benchAuth) Issue(context.Context, string, map[string]any, time.Duration) (auth.Token, error) {
	return auth.Token{}, nil
}

func (benchAuth) Verify(context.Context, string) (auth.Claims, error) {
	return auth.Claims{Subject: "user-1", Custom: map[string]any{"team": "eng"}}, nil
}

func (benchAuth) Revoke(context.Context, string) error { return nil }
func (benchAuth) Close() error                         { return nil }

type benchChecker struct{}

func (benchChecker) Can(context.Context, permission.Subject, string, permission.Resource) (permission.Decision, error) {
	return permission.Decision{Allowed: true}, nil
}

func BenchmarkAuthorize(b *testing.B) {
	a := benchAuth{}
	p := benchChecker{}
	pol := authz.Policy{
		AuthRequired:    true,
		Roles:           []string{"admin", "dev"},
		PermissionCheck: "order.read",
		ResourceType:    "Order",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := authz.Authorize(b.Context(), a, p, pol, "tok", "res-1"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAuthorizeParallel(b *testing.B) {
	a := benchAuth{}
	p := benchChecker{}
	pol := authz.Policy{
		AuthRequired:    true,
		PermissionCheck: "order.read",
		ResourceType:    "Order",
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := authz.Authorize(b.Context(), a, p, pol, "tok", "res-1"); err != nil {
				b.Error(err)
			}
		}
	})
}

func BenchmarkAuthorizeFastPath(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := authz.Authorize(b.Context(), nil, nil, authz.Policy{}, "", ""); err != nil {
			b.Fatal(err)
		}
	}
}
