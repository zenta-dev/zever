package casbin

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/permission"
)

func TestCanCancelledContextFailsClosed(t *testing.T) {
	t.Parallel()

	c, err := New(allowOpts())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	d, err := c.Can(ctx, permission.Subject{ID: "alice", Roles: []string{"admin"}}, "read", permission.Resource{Type: "doc", ID: "1"})
	if err == nil {
		t.Fatal("Can(cancelled) error = nil, want context error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Can(cancelled) error = %v, want context.Canceled", err)
	}
	if d.Allowed {
		t.Fatalf("Can(cancelled) = %+v, want denied", d)
	}
}
