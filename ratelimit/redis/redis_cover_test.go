package redis

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/zenta-dev/zever/internal/redis"
	"github.com/zenta-dev/zever/ratelimit"
)

// swapAllowScript replaces the allow script under the write lock and
// restores it on cleanup. Callers must be sequential (no t.Parallel):
// parallel tests pause until sequential tests finish, so no Allow call
// can observe the swapped script mid-flight.
func swapAllowScript(t *testing.T, src string) {
	t.Helper()

	scriptMu.Lock()
	old := allowScript
	allowScript = goredis.NewScript(src)
	scriptMu.Unlock()

	t.Cleanup(func() {
		scriptMu.Lock()
		allowScript = old
		scriptMu.Unlock()
	})
}

// deadPortLimiter builds a limiter dialing a refused port for transport
// error paths. It bypasses the shared singleton so it never thrashes it.
func deadPortLimiter(t *testing.T, burst int) *limiter {
	t.Helper()

	l := &limiter{
		client: goredis.NewClient(&goredis.Options{
			Addr:        "127.0.0.1:1",
			DialTimeout: 100 * time.Millisecond,
			ReadTimeout: 100 * time.Millisecond,
			MaxRetries:  -1,
		}),
		prefix: "cover",
		rate:   10,
		burst:  burst,
	}

	t.Cleanup(func() {
		_ = l.client.Close()
	})

	return l
}

func TestRedisCover_newDefaults(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.Redis.Prefix = ""
	// IdleTTL/SweepInterval are meaningless for Redis and must be ignored.
	opts.IdleTTL = time.Minute
	opts.SweepInterval = time.Minute

	l := newTestLimiter(t, opts)

	rl, ok := l.(*limiter)
	if !ok {
		t.Fatalf("New type = %T, want *limiter", l)
	}

	if rl.prefix != "ratelimit" {
		t.Errorf("prefix = %q, want %q", rl.prefix, "ratelimit")
	}

	if rl.rate != opts.Rate {
		t.Errorf("rate = %v, want %v", rl.rate, opts.Rate)
	}

	if rl.burst != opts.Burst {
		t.Errorf("burst = %v, want %v", rl.burst, opts.Burst)
	}

	custom := testOptions()
	custom.Redis.Prefix = "pfx"
	cl := newTestLimiter(t, custom).(*limiter) //nolint:forcetypeassert // guarded by constructor return type.

	if cl.prefix != "pfx" {
		t.Errorf("prefix = %q, want %q", cl.prefix, "pfx")
	}
}

func TestRedisCover_newPingFailRestores(t *testing.T) {
	// Sequential: New with a dead addr thrashes the shared singleton, so
	// reset it and re-New the test addr before returning.
	opts := testOptions()
	opts.Redis.Addr = "127.0.0.1:1"

	_, err := New(opts)
	if err == nil {
		t.Fatal("New(dead addr) err = nil, want ping error")
	}

	if !strings.Contains(err.Error(), "ping") {
		t.Errorf("New(dead addr) err = %v, want ping failure", err)
	}

	// The failed New closed the dead client in place; switching back to
	// the test addr forces the singleton to close it again, which fails
	// and covers the zredis.New error branch.
	_, err = New(testOptions())
	if err == nil {
		t.Fatal("New(thrash closed client) err = nil, want connect error")
	}

	if !strings.Contains(err.Error(), "connect") {
		t.Errorf("New(thrash closed client) err = %v, want connect failure", err)
	}

	// Self-restore: drop the poisoned singleton, then re-New healthy.
	_ = redis.Close()

	l, err := New(testOptions())
	if err != nil {
		t.Fatalf("New(restore) err = %v, want nil", err)
	}

	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Errorf("Close err = %v, want nil", err)
		}
	})
}

func TestRedisCover_redactAddr(t *testing.T) {
	t.Parallel()

	masked := redactAddr("redis://user:secret@host:6379")
	if strings.Contains(masked, "secret") {
		t.Errorf("redactAddr = %q, want credentials masked", masked)
	}

	if !strings.Contains(masked, "xxxxx") {
		t.Errorf("redactAddr = %q, want xxxxx mask", masked)
	}

	if got := redactAddr("127.0.0.1:6379"); got != "127.0.0.1:6379" {
		t.Errorf("redactAddr(plain) = %q, want unchanged", got)
	}

	if got := redactAddr("://bad"); got != "://bad" {
		t.Errorf("redactAddr(unparsable) = %q, want unchanged", got)
	}
}

func TestRedisCover_allowCtxCanceled(t *testing.T) {
	t.Parallel()

	l := newTestLimiter(t, testOptions())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := l.Allow(ctx, freshKey(t), 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Allow(canceled) err = %v, want context.Canceled", err)
	}
}

func TestRedisCover_allowBadCost(t *testing.T) {
	t.Parallel()

	l := newTestLimiter(t, testOptions())
	key := freshKey(t)

	for _, cost := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := l.Allow(context.Background(), key, cost); !errors.Is(err, ratelimit.ErrInvalidCost) {
			t.Errorf("Allow(%v) err = %v, want ErrInvalidCost", cost, err)
		}
	}
}

func TestRedisCover_allowOversizedCostCapped(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := newTestLimiter(t, testOptions())
	key := freshKey(t)

	// Cost above burst is capped at burst: allowed once, bucket drained.
	d, err := l.Allow(ctx, key, 10)
	if err != nil {
		t.Fatalf("Allow(oversized) err = %v, want nil", err)
	}

	if !d.Allowed {
		t.Fatal("Allow(oversized) Allowed = false, want true (capped at burst)")
	}

	if d.Remaining != 0 {
		t.Errorf("Allow(oversized) Remaining = %v, want 0 (burst drained)", d.Remaining)
	}

	if d, _ := l.Allow(ctx, key, 1); d.Allowed {
		t.Fatal("Allow after oversized Allowed = true, want false (drained)")
	}
}

func TestRedisCover_allowTransportError(t *testing.T) {
	t.Parallel()

	l := deadPortLimiter(t, 3)

	if _, err := l.Allow(context.Background(), freshKey(t), 1); err == nil {
		t.Fatal("Allow(dead port) err = nil, want transport error")
	} else if !strings.Contains(err.Error(), "redis: allow:") {
		t.Errorf("Allow(dead port) err = %v, want redis: allow: prefix", err)
	}
}

func TestRedisCover_scriptShapes(t *testing.T) {
	// Sequential (no t.Parallel): swaps allowScript under the write lock;
	// parallel Allow tests stay paused until this returns.
	ctx := context.Background()
	l := newTestLimiter(t, testOptions())

	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{name: "not-array", src: `return 1`, want: "unexpected script result"},
		{name: "allowed-type", src: `return {"x", 1, 1}`, want: "unexpected allowed type"},
		{name: "remaining-type", src: `return {1, "x", 1}`, want: "unexpected remaining type"},
		{name: "retry-type", src: `return {1, 1, "x"}`, want: "unexpected retry type"},
	} {
		swapAllowScript(t, tc.src)

		_, err := l.Allow(ctx, freshKey(t), 1)
		if err == nil {
			t.Fatalf("Allow(%s) err = nil, want %q", tc.name, tc.want)
		}

		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Allow(%s) err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestRedisCover_scriptNegativeRetryClamped(t *testing.T) {
	// Sequential: negative retryMS clamps to 0 with a denial.
	swapAllowScript(t, `return {0, 0, -5}`)

	d, err := newTestLimiter(t, testOptions()).Allow(context.Background(), freshKey(t), 1)
	if err != nil {
		t.Fatalf("Allow err = %v, want nil", err)
	}

	if d.Allowed {
		t.Fatal("Allowed = true, want false")
	}

	if d.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0 (clamped)", d.RetryAfter)
	}

	if d.Remaining != 0 {
		t.Errorf("Remaining = %v, want 0", d.Remaining)
	}
}

func TestRedisCover_resetCtxCanceled(t *testing.T) {
	t.Parallel()

	l := newTestLimiter(t, testOptions())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := l.Reset(ctx, freshKey(t)); !errors.Is(err, context.Canceled) {
		t.Errorf("Reset(canceled) err = %v, want context.Canceled", err)
	}
}

func TestRedisCover_resetDelError(t *testing.T) {
	t.Parallel()

	l := deadPortLimiter(t, 3)

	if err := l.Reset(context.Background(), freshKey(t)); err == nil {
		t.Fatal("Reset(dead port) err = nil, want transport error")
	} else if !strings.Contains(err.Error(), "redis: reset:") {
		t.Errorf("Reset(dead port) err = %v, want redis: reset: prefix", err)
	}
}

func TestRedisCover_closeNameRedisKey(t *testing.T) {
	t.Parallel()

	l := &limiter{prefix: "p"}

	if got := l.redisKey("k"); got != "p:k" {
		t.Errorf("redisKey = %q, want %q", got, "p:k")
	}

	if got := l.Name(); got != "redis" {
		t.Errorf("Name = %q, want %q", got, "redis")
	}

	if err := l.Close(); err != nil {
		t.Errorf("Close err = %v, want nil", err)
	}

	if err := l.Close(); err != nil {
		t.Errorf("Close second err = %v, want nil", err)
	}
}
