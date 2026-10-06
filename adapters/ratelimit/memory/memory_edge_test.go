package memory_test

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/adapters/ratelimit/memory"
	"github.com/zenta-dev/zever/core/ratelimit"
)

func TestAllow_canceledContext(t *testing.T) {
	t.Parallel()

	l, err := memory.New(ratelimit.Options{Rate: 10, Burst: 5})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	defer func() { _ = l.Close() }()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := l.Allow(ctx, "k", 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Allow(canceled) = %v, want context.Canceled", err)
	}
}

func TestAllow_costAboveBurst_capped(t *testing.T) {
	t.Parallel()

	l, err := memory.New(ratelimit.Options{Rate: 1, Burst: 3})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	defer func() { _ = l.Close() }()

	d, err := l.Allow(t.Context(), "k", 10)
	if err != nil {
		t.Fatalf("Allow(cost>burst) = %v", err)
	}
	if !d.Allowed {
		t.Error("Allow(cost>burst) Allowed = false, want true (cost capped at burst)")
	}
	if d.Remaining != 0 {
		t.Errorf("Remaining = %v, want 0", d.Remaining)
	}
}

func TestName(t *testing.T) {
	t.Parallel()

	l, err := memory.New(ratelimit.Options{Rate: 1, Burst: 1})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	defer func() { _ = l.Close() }()

	if got := l.Name(); got != "memory" {
		t.Errorf("Name() = %q, want memory", got)
	}
}

// TestRegisterOpensViaCoreOptions proves Register wires the adapter factory
// into the ratelimit battery registry so ratelimit.Open resolves it.
func TestRegisterOpensViaCoreOptions(t *testing.T) {
	memory.Register()

	l, err := ratelimit.Open(ratelimit.Memory, ratelimit.Options{Rate: 1, Burst: 1})
	if err != nil {
		t.Fatalf("Open = %v", err)
	}

	t.Cleanup(func() { _ = l.Close() })

	if d, allowErr := l.Allow(t.Context(), "k", 1); allowErr != nil || !d.Allowed {
		t.Fatalf("Allow = (%+v, %v), want allowed", d, allowErr)
	}
}
