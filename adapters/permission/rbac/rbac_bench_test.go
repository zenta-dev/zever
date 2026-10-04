package rbac_test

import (
	"testing"

	"github.com/zenta-dev/zever/adapters/permission/rbac"
	"github.com/zenta-dev/zever/core/permission"
)

// BenchmarkCan_deny measures the explicit-deny hot path (deny scan runs first).
func BenchmarkCan_deny(b *testing.B) {
	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{
			{Role: "admin", Action: "*"},
			{Role: "admin", Action: "read", Effect: permission.Deny},
		},
	})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	sub := permission.Subject{ID: "u1", Roles: []string{"admin"}}
	res := permission.Resource{Type: "doc", ID: "d1"}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := c.Can(ctx, sub, "read", res); err != nil {
			b.Fatalf("Can() = %v", err)
		}
	}
}

// BenchmarkCanParallel measures concurrent checks against one read-only checker.
func BenchmarkCanParallel(b *testing.B) {
	c, err := rbac.New(permission.Options{
		Rules: []permission.Rule{{Role: "admin", Action: "read"}},
	})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	sub := permission.Subject{ID: "u1", Roles: []string{"admin"}}
	res := permission.Resource{Type: "doc", ID: "d1"}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := c.Can(ctx, sub, "read", res); err != nil {
				b.Errorf("Can() = %v", err)
				return
			}
		}
	})
}
