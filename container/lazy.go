package container

import (
	"runtime/debug"
	"sync"
	"sync/atomic"
)

// lazy caches fn's successful result for subsequent get calls.
//
// lazy is sealed to this package via the unexported sealed marker, so
// external packages can neither instantiate nor embed it.
//
// A failed build is not cached: the stored value resets to zero and a later
// get retries. Concurrent callers share a single in-flight execution
// (singleflight) instead of each invoking fn, and callers parked on a flight
// receive that flight's own result, value or error (a panic arrives as a
// BuildPanicError). Only a call that starts after the flight settled retries,
// so a deterministically failing builder runs once per flight, not once per
// waiter. A flight abandoned by runtime.Goexit has no result to share, so its
// waiters retry.
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
	// done guards success under mu; ready publishes it lock-free.
	done  bool
	ready atomic.Bool
	// cur is the in-flight build, nil when none is running.
	cur *flight[T]
	// parked counts callers that joined a flight as waiters (test hook).
	parked int
}

// flight is one build execution. val, err and settled are written by the
// leader before done is closed, so waiters read them after <-done without
// further locking.
type flight[T any] struct {
	done    chan struct{}
	val     T
	err     error
	settled bool // false when the leader exited via runtime.Goexit
}

// sealed marks lazy as package-private.
func (l *lazy[T]) sealed() {}

// get returns the cached value, invoking fn once to build it. Builders carry
// no caller context: lazy accessors have none to pass, and builders that need
// one use Container.rootCtx.
func (l *lazy[T]) get(fn func() (T, error)) (T, error) {
	// Fast path: ready is monotonic, so observing true means val is fully
	// published and immutable; no mutex needed.
	if l.ready.Load() {
		return l.val, nil
	}

	l.mu.Lock()

	for {
		if l.done {
			v := l.val
			l.mu.Unlock()

			return v, nil
		}

		f := l.cur
		if f == nil {
			break
		}

		l.parked++
		l.mu.Unlock()

		<-f.done

		if f.settled {
			return f.val, f.err
		}

		// Leader Goexited: nothing to share, contend to lead a retry.
		l.mu.Lock()
	}

	f := &flight[T]{done: make(chan struct{})}
	l.cur = f
	l.mu.Unlock()

	return l.leaderBuild(f, fn)
}

// leaderBuild runs fn outside mu so waiters can park instead of serializing
// behind the build. Whether fn returns normally, panics, or calls
// runtime.Goexit, the deferred cleanup always clears l.cur, releases mu, and
// closes f.done, so parked waiters never wedge.
//
// The defer distinguishes the three exits: a recovered panic becomes a
// BuildPanicError; returned is set only when fn returns normally, so a
// Goexit — which runs defers but skips the rest of the body — releases the
// waiters unsettled and leaves the build uncached (recover returns nil for
// Goexit, so no panic error is recorded) instead of caching a zero value.
func (l *lazy[T]) leaderBuild(f *flight[T], fn func() (T, error)) (v T, err error) {
	returned := false

	defer func() {
		panicked := false

		if r := recover(); r != nil {
			err = BuildPanicError{Value: r, Stack: debug.Stack()}
			panicked = true
		}

		l.mu.Lock()

		if returned && err == nil {
			l.val = v
			l.done = true
			// Publish while still holding mu so the val write is ordered
			// before this Store; any later Load observing true synchronizes
			// with it. Monotonic: never store false.
			l.ready.Store(true)
		} else {
			var zero T

			v = zero
		}

		f.val, f.err, f.settled = v, err, returned || panicked
		l.cur = nil
		l.mu.Unlock()

		close(f.done)
	}()

	v, err = fn()
	returned = true

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
