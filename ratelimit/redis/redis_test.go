package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/ratelimit"
)

// Shared hermetic server: internal/redis Pool singleton means all tests
// must dial the same address; isolation comes from unique keys per subtest.
var (
	testMini *miniredis.Miniredis
	testAddr string
	keySeq   atomic.Int64
)

func TestMain(m *testing.M) {
	s, err := miniredis.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "miniredis start:", err)
		os.Exit(1)
	}

	testMini = s
	testAddr = s.Addr()

	code := m.Run()

	s.Close()

	os.Exit(code)
}

func freshKey(t *testing.T) string {
	t.Helper()

	return fmt.Sprintf("k-%d", keySeq.Add(1))
}

func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", msg)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

func testOptions() ratelimit.Options {
	return ratelimit.Options{
		Rate:  10,
		Burst: 3,
		Redis: ratelimit.RedisOptions{Addr: testAddr},
	}
}

func newTestLimiter(t *testing.T, opts ratelimit.Options) ratelimit.Limiter {
	t.Helper()

	l, err := New(opts)
	if err != nil {
		t.Fatalf("New err = %v, want nil", err)
	}

	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Errorf("Close err = %v, want nil", err)
		}
	})

	return l
}

func TestRedis_allowBurstThenDeny(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := newTestLimiter(t, testOptions())
	key := freshKey(t)

	for i := 0; i < 3; i++ {
		d, err := l.Allow(ctx, key, 1)
		if err != nil {
			t.Fatalf("Allow %d err = %v, want nil", i, err)
		}

		if !d.Allowed {
			t.Fatalf("Allow %d Allowed = false, want true", i)
		}
	}

	d, err := l.Allow(ctx, key, 1)
	if err != nil {
		t.Fatalf("Allow over burst err = %v, want nil", err)
	}

	if d.Allowed {
		t.Fatalf("Allow over burst Allowed = true, want false")
	}

	if d.RetryAfter <= 0 {
		t.Fatalf("RetryAfter = %v, want > 0", d.RetryAfter)
	}

	if d.Remaining != 0 {
		t.Fatalf("Remaining = %v, want 0", d.Remaining)
	}
}

func TestRedis_refill(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	opts := testOptions()
	opts.Rate = 5
	opts.Burst = 2
	l := newTestLimiter(t, opts)
	key := freshKey(t)

	for i := 0; i < 2; i++ {
		if d, err := l.Allow(ctx, key, 1); err != nil || !d.Allowed {
			t.Fatalf("Allow %d = %+v, %v; want allowed", i, d, err)
		}
	}

	if d, _ := l.Allow(ctx, key, 1); d.Allowed {
		t.Fatalf("Allow over burst Allowed = true, want false")
	}

	// Rate 5/s refills one token per 200ms: poll until the refill lands.
	eventually(t, 5*time.Second, func() bool {
		d, err := l.Allow(ctx, key, 1)
		return err == nil && d.Allowed
	}, "token refill")
}

func TestRedis_invalidCost(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := newTestLimiter(t, testOptions())
	key := freshKey(t)

	for _, cost := range []float64{0, -1} {
		if _, err := l.Allow(ctx, key, cost); !errors.Is(err, ratelimit.ErrInvalidCost) {
			t.Errorf("Allow(%v) err = %v, want ErrInvalidCost", cost, err)
		}
	}
}

func TestRedis_invalidKey(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := newTestLimiter(t, testOptions())

	long := make([]byte, ratelimit.MaxKeyLen+1)
	for i := range long {
		long[i] = 'k'
	}

	bad := map[string]string{
		"empty":   "",
		"newline": "a\nb",
		"control": "a\x00b",
		"toolong": string(long),
	}

	for name, key := range bad {
		if _, err := l.Allow(ctx, key, 1); !errors.Is(err, ratelimit.ErrInvalidKey) {
			t.Errorf("Allow(%s) err = %v, want ErrInvalidKey", name, err)
		}

		if err := l.Reset(ctx, key); !errors.Is(err, ratelimit.ErrInvalidKey) {
			t.Errorf("Reset(%s) err = %v, want ErrInvalidKey", name, err)
		}
	}
}

func TestRedis_resetRestores(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := newTestLimiter(t, testOptions())
	key := freshKey(t)

	for i := 0; i < 3; i++ {
		if _, err := l.Allow(ctx, key, 1); err != nil {
			t.Fatalf("Allow err = %v, want nil", err)
		}
	}

	if d, _ := l.Allow(ctx, key, 1); d.Allowed {
		t.Fatalf("Allow over burst Allowed = true, want false")
	}

	if err := l.Reset(ctx, key); err != nil {
		t.Fatalf("Reset err = %v, want nil", err)
	}

	d, err := l.Allow(ctx, key, 1)
	if err != nil {
		t.Fatalf("Allow after Reset err = %v, want nil", err)
	}

	if !d.Allowed {
		t.Fatalf("Allow after Reset Allowed = false, want true")
	}

	if err := l.Reset(ctx, freshKey(t)); err != nil {
		t.Fatalf("Reset missing err = %v, want nil", err)
	}
}

func TestRedis_prefixIsolation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	optsA := testOptions()
	optsA.Redis.Prefix = "pfxA"
	optsB := testOptions()
	optsB.Redis.Prefix = "pfxB"

	a := newTestLimiter(t, optsA)
	b := newTestLimiter(t, optsB)
	key := freshKey(t)

	// Drain A fully.
	for i := 0; i < 3; i++ {
		if d, err := a.Allow(ctx, key, 1); err != nil || !d.Allowed {
			t.Fatalf("A Allow %d = %+v, %v; want allowed", i, d, err)
		}
	}

	if d, _ := a.Allow(ctx, key, 1); d.Allowed {
		t.Fatalf("A over burst Allowed = true, want false")
	}

	// B must be unaffected.
	d, err := b.Allow(ctx, key, 1)
	if err != nil {
		t.Fatalf("B Allow err = %v, want nil", err)
	}

	if !d.Allowed {
		t.Fatalf("B Allow Allowed = false, want true (isolated prefix)")
	}
}

func TestRedis_afterClose(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	l := newTestLimiter(t, testOptions())
	key := freshKey(t)

	if err := l.Close(); err != nil {
		t.Fatalf("Close err = %v, want nil", err)
	}

	if err := l.Close(); err != nil {
		t.Fatalf("Close second err = %v, want nil", err)
	}

	if _, err := l.Allow(ctx, key, 1); !errors.Is(err, ratelimit.ErrClosed) {
		t.Fatalf("Allow after Close err = %v, want ErrClosed", err)
	}

	if err := l.Reset(ctx, key); !errors.Is(err, ratelimit.ErrClosed) {
		t.Fatalf("Reset after Close err = %v, want ErrClosed", err)
	}
}

func TestRedis_concurrentSingleBucketBounded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	opts := testOptions()
	opts.Rate = 1
	opts.Burst = 5
	l := newTestLimiter(t, opts)
	key := freshKey(t)

	const n = 20

	var wg sync.WaitGroup

	allowed := atomic.Int64{}

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			d, err := l.Allow(ctx, key, 1)
			if err != nil {
				t.Errorf("Allow err = %v, want nil", err)

				return
			}

			if d.Allowed {
				allowed.Add(1)
			}
		}()
	}

	wg.Wait()

	// Burst 5 + ~1/s refill over the short race window: allow small slack.
	if got := allowed.Load(); got > 7 {
		t.Fatalf("allowed = %d, want <= 7 (burst + refill bound)", got)
	}

	if got := allowed.Load(); got == 0 {
		t.Fatalf("allowed = 0, want > 0")
	}
}

func TestRedis_invalidOptions(t *testing.T) {
	t.Parallel()

	for name, mutate := range map[string]func(*ratelimit.Options){
		"bad rate":   func(o *ratelimit.Options) { o.Rate = 0 },
		"bad burst":  func(o *ratelimit.Options) { o.Burst = 0 },
		"bad addr":   func(o *ratelimit.Options) { o.Redis.Addr = "://bad" },
		"bad prefix": func(o *ratelimit.Options) { o.Redis.Prefix = "has space" },
	} {
		opts := testOptions()
		mutate(&opts)

		if _, err := New(opts); err == nil {
			t.Errorf("New(%s) err = nil, want error", name)
		}
	}
}
