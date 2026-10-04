package casbin

import (
	"fmt"
	"testing"

	"github.com/zenta-dev/zever/core/permission"
)

func benchChecker(b *testing.B) permission.Checker {
	b.Helper()
	c, err := New(allowOpts())
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	return c
}

// BenchmarkCan measures the enforce hot path including transient grouping
// add/cleanup.
func BenchmarkCan(b *testing.B) {
	c := benchChecker(b)
	sub := permission.Subject{ID: "alice", Roles: []string{"admin"}}
	res := permission.Resource{Type: "doc", ID: "1"}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Can(ctx, sub, "read", res); err != nil {
			b.Fatalf("Can() = %v", err)
		}
	}
}

// BenchmarkCanParallel measures concurrent enforce calls with distinct
// subjects (each exercises its own grouping ref count).
func BenchmarkCanParallel(b *testing.B) {
	opts := permission.Options{Rules: []permission.Rule{{Role: "admin", Action: "read"}}}
	opts.Roles = make(map[string][]string)
	for i := range 16 {
		opts.Roles[fmt.Sprintf("user-%d", i)] = []string{"admin"}
	}
	c, err := New(opts)
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	res := permission.Resource{Type: "doc", ID: "1"}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			sub := permission.Subject{ID: fmt.Sprintf("user-%d", i%16), Roles: []string{"admin"}}
			i++
			if _, err := c.Can(ctx, sub, "read", res); err != nil {
				b.Errorf("Can() = %v", err)
				return
			}
		}
	})
}
