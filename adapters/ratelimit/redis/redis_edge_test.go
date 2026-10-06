package redis

import (
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/ratelimit"
)

func TestRedis_costExactlyBurst_allowed(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	opts.Burst = 3
	l := newTestLimiter(t, opts)
	key := freshKey(t)

	d, err := l.Allow(ctx, key, float64(opts.Burst))
	if err != nil {
		t.Fatalf("Allow(cost==burst) = %v", err)
	}
	if !d.Allowed {
		t.Errorf("Allow(cost==burst) Allowed = false, want true")
	}
	if d.Remaining != 0 {
		t.Errorf("Remaining = %v, want 0", d.Remaining)
	}
}

func TestRedis_concurrentDistinctKeys_allAllowed(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	opts.Rate = 1e9
	opts.Burst = 1000
	l := newTestLimiter(t, opts)

	const workers = 32

	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := l.Allow(ctx, freshKey(t), 1)
			if err != nil || !d.Allowed {
				t.Errorf("worker %d Allow() = (%+v, %v), want allowed", w, d, err)
			}
		}()
	}
	wg.Wait()
}

func TestRedis_ResetThenAllow(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	opts := testOptions(t)
	opts.Burst = 1
	l := newTestLimiter(t, opts)
	key := freshKey(t)

	if d, err := l.Allow(ctx, key, 1); err != nil || !d.Allowed {
		t.Fatalf("first Allow() = (%+v, %v), want allowed", d, err)
	}
	if d, _ := l.Allow(ctx, key, 1); d.Allowed {
		t.Fatal("second Allow() = allowed, want denied")
	}
	if err := l.Reset(ctx, key); err != nil {
		t.Fatalf("Reset() = %v", err)
	}
	if d, err := l.Allow(ctx, key, 1); err != nil || !d.Allowed {
		t.Errorf("Allow(after Reset) = (%+v, %v), want allowed", d, err)
	}
}

func TestRedis_Name(t *testing.T) {
	t.Parallel()

	l := newTestLimiter(t, testOptions(t))
	if got := l.Name(); got != "redis" {
		t.Errorf("Name() = %q, want redis", got)
	}
}

// TestRegisterOpensViaCoreOptions proves Register wires the adapter factory
// into the ratelimit battery registry so ratelimit.Open resolves it.
// Sequential by design: New threads through the shared client singleton.
func TestRegisterOpensViaCoreOptions(t *testing.T) {
	s := testServer(t)

	Register()

	l, err := ratelimit.Open(ratelimit.Redis, optionsFor(s))
	if err != nil {
		t.Fatalf("Open = %v", err)
	}

	t.Cleanup(func() { _ = l.Close() })

	if d, allowErr := l.Allow(t.Context(), "k", 1); allowErr != nil || !d.Allowed {
		t.Fatalf("Allow = (%+v, %v), want allowed", d, allowErr)
	}
}
