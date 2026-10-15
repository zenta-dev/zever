package container

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/ratelimit"
)

// countingLimiter fails the test if used after Close and counts Close calls.
type countingLimiter struct {
	closes atomic.Int32
	usedAf atomic.Int32
}

var _ ratelimit.Limiter = (*countingLimiter)(nil)

func (l *countingLimiter) Allow(context.Context, string, float64) (ratelimit.Decision, error) {
	if l.closes.Load() > 0 {
		l.usedAf.Add(1)
	}

	return ratelimit.Decision{Allowed: true}, nil
}

func (l *countingLimiter) Reset(context.Context, string) error { return nil }
func (l *countingLimiter) Close() error                        { l.closes.Add(1); return nil }
func (l *countingLimiter) Name() string                        { return "counting" }

// blockingLimiter parks Allow until released.
type blockingLimiter struct {
	countingLimiter
	entered chan struct{}
	release chan struct{}
}

func (l *blockingLimiter) Allow(ctx context.Context, k string, n float64) (ratelimit.Decision, error) {
	close(l.entered)
	<-l.release

	return l.countingLimiter.Allow(ctx, k, n)
}

func TestReloadableLimiterConcurrentAllowDuringSwaps(t *testing.T) {
	initial := &countingLimiter{}
	r := NewReloadableLimiter(initial)

	var (
		mu       sync.Mutex
		opened   = make([]*countingLimiter, 1, 201)
		wg       sync.WaitGroup
		stopFlag atomic.Bool
	)

	opened[0] = initial

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for !stopFlag.Load() {
				if _, err := r.Allow(t.Context(), "k", 1); err != nil {
					t.Error(err)
				}
			}
		}()
	}

	for range 200 {
		next := &countingLimiter{}

		mu.Lock()
		opened = append(opened, next)
		mu.Unlock()

		r.Swap(next)
	}

	stopFlag.Store(true)
	wg.Wait()

	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	for i, l := range opened {
		want := int32(1)
		if i == 0 {
			want = 0 // borrowed initial: owner closes it
		}

		if got := l.closes.Load(); got != want {
			t.Errorf("limiter %d closed %d times, want %d", i, got, want)
		}

		if got := l.usedAf.Load(); got != 0 {
			t.Errorf("limiter %d used %d times after Close", i, got)
		}
	}
}

func TestReloadableLimiterDefersCloseUntilInflightDrains(t *testing.T) {
	old := &blockingLimiter{entered: make(chan struct{}), release: make(chan struct{})}
	r := NewReloadableLimiter(&countingLimiter{})
	r.Swap(old) // old is now owned

	done := make(chan struct{})

	go func() {
		defer close(done)

		_, _ = r.Allow(t.Context(), "k", 1)
	}()

	<-old.entered

	next := &countingLimiter{}
	r.Swap(next)

	if got := old.closes.Load(); got != 0 {
		t.Fatalf("old limiter closed %d times while Allow in flight", got)
	}

	close(old.release)
	<-done

	if got := old.closes.Load(); got != 1 {
		t.Errorf("old limiter closes after drain = %d, want 1", got)
	}
}

func TestReloadableLimiterShutdownClosesEachOnce(t *testing.T) {
	initial := &countingLimiter{}
	r := NewReloadableLimiter(initial)
	second := &countingLimiter{}
	r.Swap(second)

	// The container closes the borrowed initial at shutdown; the
	// reloadable closes what it owns. Close twice to prove idempotence.
	_ = initial.Close()

	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	if got := initial.closes.Load(); got != 1 {
		t.Errorf("initial closes = %d, want 1", got)
	}

	if got := second.closes.Load(); got != 1 {
		t.Errorf("second closes = %d, want 1", got)
	}

	if _, swapped := r.Swap(&countingLimiter{}); swapped {
		t.Error("Swap after Close succeeded")
	}

	// Allow after Close must not spin.
	_, _ = r.Allow(t.Context(), "k", 1)
}

func TestReloadableLoggerChildFollowsSwap(t *testing.T) {
	r := NewReloadableLogger(&stubLogger{min: log.LevelInfo})
	child := r.With().Str("a", "b").Logger()
	grand := child.With().Int("n", 1).Logger()
	ctxChild := r.WithContext(t.Context())

	for name, l := range map[string]log.Logger{"child": child, "grand": grand, "ctx": ctxChild} {
		if l.Enabled(log.LevelDebug) {
			t.Fatalf("%s: debug enabled before swap", name)
		}
	}

	r.Swap(&stubLogger{min: log.LevelDebug})

	for name, l := range map[string]log.Logger{"child": child, "grand": grand, "ctx": ctxChild} {
		if !l.Enabled(log.LevelDebug) {
			t.Errorf("%s: level change not followed", name)
		}
	}
}
