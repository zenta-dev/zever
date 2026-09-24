package container

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// eventually polls cond until true or timeout, failing the test on expiry.
func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("eventually timed out after %v: %s", timeout, msg)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestLazy_SuccessCachesValue(t *testing.T) {
	var l lazy[int]
	var calls atomic.Int32
	build := func() (int, error) {
		calls.Add(1)
		return 7, nil
	}

	for i := 0; i < 100; i++ {
		v, err := l.get(build)
		if err != nil {
			t.Fatalf("get %d: unexpected error: %v", i, err)
		}
		if v != 7 {
			t.Fatalf("get %d: got %d, want 7", i, v)
		}
	}

	if calls.Load() != 1 {
		t.Fatalf("build ran %d times, want 1", calls.Load())
	}
	if !l.resolved() {
		t.Fatal("resolved is false after success")
	}
	if v, ok := l.getIfResolved(); !ok || v != 7 {
		t.Fatalf("getIfResolved = (%d, %v), want (7, true)", v, ok)
	}
	l.sealed()
}

func TestLazy_FailureRetriesUntilSuccess(t *testing.T) {
	var l lazy[int]
	fail := errors.New("build failed")
	calls := 0
	build := func() (int, error) {
		calls++
		if calls < 3 {
			return 0, fail
		}
		return 42, nil
	}

	for i := 1; i <= 2; i++ {
		if _, err := l.get(build); !errors.Is(err, fail) {
			t.Fatalf("get %d: got %v, want build failure", i, err)
		}
		if l.resolved() {
			t.Fatalf("get %d: resolved is true after failure", i)
		}
	}

	v, err := l.get(build)
	if err != nil {
		t.Fatalf("final get: unexpected error: %v", err)
	}
	if v != 42 {
		t.Fatalf("final get: got %d, want 42", v)
	}
	if calls != 3 {
		t.Fatalf("build ran %d times, want 3", calls)
	}
	if !l.resolved() {
		t.Fatal("resolved is false after success")
	}

	// Success is cached: no further builds.
	if _, err := l.get(build); err != nil {
		t.Fatalf("cached get: unexpected error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("build ran %d times after cache, want 3", calls)
	}
}

func TestLazy_ConcurrentSingleflight(t *testing.T) {
	var l lazy[string]
	var calls atomic.Int32
	//nolint:unparam // get requires (T, error); the shared-success path is nil by design
	build := func() (string, error) {
		calls.Add(1)
		// Simulated work to widen the singleflight overlap window, not a
		// sync wait: start-chan + WaitGroup below provide the synchronization.
		time.Sleep(50 * time.Millisecond)
		return "shared", nil
	}

	const n = 50
	start := make(chan struct{})
	vals := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v, err := l.get(build)
			// Each goroutine writes its own index: no shared mutation.
			vals[i], errs[i] = v, err
		}()
	}
	close(start)
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		if vals[i] != "shared" {
			t.Fatalf("goroutine %d: got %q, want %q", i, vals[i], "shared")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("build ran %d times, want 1", calls.Load())
	}
}

func TestLazy_WaiterBecomesRetryLeader(t *testing.T) {
	var l lazy[int]
	fail := errors.New("leader failed")
	release := make(chan struct{})
	entered := make(chan struct{})
	leaderBuild := func() (int, error) {
		close(entered)
		<-release
		return 0, fail
	}
	var waiterCalls atomic.Int32
	//nolint:unparam // get requires (T, error); the retry-success path is nil by design
	waiterBuild := func() (int, error) {
		waiterCalls.Add(1)
		return 9, nil
	}

	var wg sync.WaitGroup
	wg.Add(2)
	var leaderVal, waiterVal int
	var leaderErr, waiterErr error
	var waiterStarted atomic.Bool
	go func() {
		defer wg.Done()
		leaderVal, leaderErr = l.get(leaderBuild)
	}()
	<-entered
	go func() {
		defer wg.Done()
		waiterStarted.Store(true)
		waiterVal, waiterErr = l.get(waiterBuild)
	}()
	// Wait until the waiter goroutine has started instead of a fixed sleep.
	// Even if it arrives late it becomes the retry leader itself; the
	// assertions hold either way.
	eventually(t, 2*time.Second, waiterStarted.Load, "waiter goroutine did not start")
	close(release)
	wg.Wait()

	if !errors.Is(leaderErr, fail) {
		t.Fatalf("leader: got (%d, %v), want failure", leaderVal, leaderErr)
	}
	if waiterErr != nil || waiterVal != 9 {
		t.Fatalf("waiter: got (%d, %v), want (9, nil)", waiterVal, waiterErr)
	}
	if waiterCalls.Load() != 1 {
		t.Fatalf("waiter build ran %d times, want 1", waiterCalls.Load())
	}
	if !l.resolved() {
		t.Fatal("resolved is false after waiter retry succeeded")
	}
}

func TestLazy_ResolvedAndGetIfResolvedDoNotTrigger(t *testing.T) {
	var l lazy[int]
	if l.resolved() {
		t.Fatal("resolved is true on zero value")
	}
	if v, ok := l.getIfResolved(); ok || v != 0 {
		t.Fatalf("getIfResolved = (%d, %v), want (0, false)", v, ok)
	}

	build := func() (int, error) {
		return 3, nil
	}
	if _, err := l.get(build); err != nil {
		t.Fatalf("get: unexpected error: %v", err)
	}
	if !l.resolved() {
		t.Fatal("resolved is false after success")
	}
	if v, ok := l.getIfResolved(); !ok || v != 3 {
		t.Fatalf("getIfResolved = (%d, %v), want (3, true)", v, ok)
	}
}

func TestLazy_FailureStoresZeroValue(t *testing.T) {
	var l lazy[string]
	fail := errors.New("build failed")
	if _, err := l.get(func() (string, error) { return "partial", fail }); !errors.Is(err, fail) {
		t.Fatalf("get: got %v, want build failure", err)
	}
	if l.val != "" {
		t.Fatalf("stored value = %q, want zero after failure", l.val)
	}
	if v, ok := l.getIfResolved(); ok || v != "" {
		t.Fatalf("getIfResolved = (%q, %v), want (\"\", false)", v, ok)
	}
	if l.resolved() {
		t.Fatal("resolved is true after failure")
	}
}

func TestLazy_DoneWithoutReadyReturnsCached(t *testing.T) {
	// White-box: done set without ready publication takes the mutex-guarded
	// path instead of the atomic fast path.
	var l lazy[int]
	l.mu.Lock()
	l.val = 5
	l.done = true
	l.mu.Unlock()

	v, err := l.get(func() (int, error) {
		t.Error("build must not run when done is set")
		return -1, errors.New("unreachable")
	})
	if err != nil {
		t.Fatalf("get: unexpected error: %v", err)
	}
	if v != 5 {
		t.Fatalf("get = %d, want 5", v)
	}
}

func TestLazy_WaiterDoneBranch(t *testing.T) {
	// White-box: a waiter that wakes with done set but ready unset returns
	// via the in-loop done check. Repeated so the waiter reliably parks
	// before the setter runs; coverage accumulates across iterations.
	for i := 0; i < 10; i++ {
		var l lazy[int]
		ch := make(chan struct{})
		l.mu.Lock()
		l.wait = ch
		l.mu.Unlock()

		finished := make(chan struct{})
		started := make(chan struct{})
		go func() {
			defer close(finished)
			close(started)
			v, err := l.get(func() (int, error) {
				return -1, errors.New("build must not run")
			})
			if err != nil {
				t.Errorf("iter %d: unexpected error: %v", i, err)
			}
			if v != 7 {
				t.Errorf("iter %d: got %d, want 7", i, v)
			}
		}()
		// Wait for the waiter goroutine to start (channel sync the lazy
		// wait-channel already exposes) instead of a fixed sleep. If the
		// setter wins the race the waiter takes the mutex-guarded done
		// path; assertions still hold and coverage accumulates across
		// iterations.
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("iter %d: waiter goroutine did not start", i)
		}
		l.mu.Lock()
		l.val = 7
		l.done = true
		l.wait = nil
		l.mu.Unlock()
		close(ch)
		<-finished
	}
}
