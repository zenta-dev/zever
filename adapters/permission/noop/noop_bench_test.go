package noop_test

import (
	"testing"

	"github.com/zenta-dev/zever/adapters/permission/noop"
	"github.com/zenta-dev/zever/core/permission"
)

func benchChecker(b *testing.B) permission.Checker {
	b.Helper()
	c, err := noop.New(permission.Options{})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	return c
}

// BenchmarkCan measures the deny-all hot path.
func BenchmarkCan(b *testing.B) {
	c := benchChecker(b)
	sub := permission.Subject{ID: "u1", Roles: []string{"admin"}}
	res := permission.Resource{Type: "doc", ID: "d1"}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Can(ctx, sub, "read", res); err != nil {
			b.Fatalf("Can() = %v", err)
		}
	}
}

// BenchmarkCanParallel measures concurrent deny-all checks.
func BenchmarkCanParallel(b *testing.B) {
	c := benchChecker(b)
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
