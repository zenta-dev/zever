package container

import (
	"sync"
	"sync/atomic"
)

// lazy caches fn's successful result for subsequent get calls.
//
// lazy is sealed to this package via the unexported sealed marker, so
// external packages can neither instantiate nor embed it.
//
// A failed build is not cached: the stored value resets to zero and the
// next get retries. Concurrent callers share a single in-flight execution
// (singleflight) instead of each invoking fn.
//
// ready is an atomic publication flag layered over the mutex-guarded state.
// The leader stores ready=true while still holding mu, after writing val;
// any goroutine that observes ready==true therefore also observes the
// earlier plain write to val (sync/atomic operations are sequentially
// consistent, so the Store synchronizes-before the observing Load under the
// Go memory model). ready moves false→true at most once and is never reset,
// so once observed true, val is immutable and get can return it without
// taking mu — the steady-state fast path is a single atomic load.
//
// A hand-rolled flag is used instead of sync.Once/sync.OnceValues because
// those run their function at most once ever and cache failures (Go issue
// #22098): they cannot express retry-after-failure without reintroducing
// this same bookkeeping.
type lazy[T any] struct {
	mu  sync.Mutex
	val T
	err error
	// done guards success under mu; ready publishes it lock-free.
	done  bool
	ready atomic.Bool
	wait  chan struct{}
}

// sealed marks lazy as package-private.
func (l *lazy[T]) sealed() {} //nolint:unused // seal marker

// get returns the cached value, invoking fn once to build it.
func (l *lazy[T]) get(fn func() (T, error)) (T, error) {
	// Fast path: ready is monotonic, so observing true means val is fully
	// published and immutable; no mutex needed.
	if l.ready.Load() {
		return l.val, nil
	}

	l.mu.Lock()

	if l.done {
		v, e := l.val, l.err
		l.mu.Unlock()

		return v, e
	}

	// Singleflight wait loop: waiters re-check after each wake. A waiter
	// that wakes with no retry in flight falls through and becomes the
	// retry leader itself, so waiters never wedge when a retry also fails.
	for l.wait != nil {
		ch := l.wait
		l.mu.Unlock()

		<-ch

		if l.ready.Load() {
			return l.val, nil
		}

		l.mu.Lock()

		if l.done {
			v, e := l.val, l.err
			l.mu.Unlock()

			return v, e
		}
	}

	l.wait = make(chan struct{})
	l.mu.Unlock()

	// Leader runs fn outside mu so waiters can park instead of serializing
	// behind the build.
	v, err := fn()

	l.mu.Lock()

	if err == nil {
		l.val = v
		l.err = nil
		l.done = true
		// Publish while still holding mu so the val write is ordered
		// before this Store; any later Load observing true synchronizes
		// with it. Monotonic: never store false.
		l.ready.Store(true)
	} else {
		var zero T

		l.val = zero
		l.err = err
	}

	ch := l.wait
	l.wait = nil
	l.mu.Unlock()

	close(ch)

	return v, err
}

// resolved reports whether get has completed successfully, without running fn.
func (l *lazy[T]) resolved() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.done
}

// getIfResolved returns the cached value without running fn.
func (l *lazy[T]) getIfResolved() (T, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.done {
		var zero T
		return zero, false
	}

	return l.val, true
}
