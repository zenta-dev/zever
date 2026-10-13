package container

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/shared/grpcclient"
)

// eventually polls cond until true or timeout, failing the test on expiry.
func eventually(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("eventually timed out after %v: %s", timeout, msg)
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("eventually aborted: test context done: %s", msg)
		case <-timer.C:
		}
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

func TestLazy_PanicReleasesWaiters(t *testing.T) {
	const panicVal = "boom"
	//nolint:unparam // get requires (T, error); panic path is nil by design
	build := func() (int, error) {
		panic(panicVal)
	}

	const n = 8
	var l lazy[int]
	start := make(chan struct{})
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := l.get(build)
			errs[i] = err
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		var buildErr BuildPanicError
		if !errors.As(err, &buildErr) {
			t.Fatalf("waiter %d: got %T (%v), want BuildPanicError", i, err, err)
		}
		if buildErr.Value != panicVal {
			t.Errorf("waiter %d: panic value = %v, want %v", i, buildErr.Value, panicVal)
		}
		if len(buildErr.Stack) == 0 {
			t.Errorf("waiter %d: stack is empty", i)
		}
	}
	if l.resolved() {
		t.Fatal("resolved is true after panic")
	}
}

func TestLazy_PanicRetriesAndSucceeds(t *testing.T) {
	var l lazy[int]
	var calls atomic.Int32
	fail := errors.New("build failed")
	//nolint:unparam // get requires (T, error); mixed panic/error path is nil by design
	build := func() (int, error) {
		switch calls.Add(1) {
		case 1:
			panic("first build panics")
		case 2:
			return 0, fail
		default:
			return 42, nil
		}
	}

	_, err := l.get(build)
	var buildErr BuildPanicError
	if !errors.As(err, &buildErr) {
		t.Fatalf("first get: got %T (%v), want BuildPanicError", err, err)
	}
	if l.resolved() {
		t.Fatal("resolved is true after panic")
	}

	_, err = l.get(build)
	if !errors.Is(err, fail) {
		t.Fatalf("second get: got %v, want build failure", err)
	}

	v, err := l.get(build)
	if err != nil {
		t.Fatalf("third get: unexpected error: %v", err)
	}
	if v != 42 {
		t.Fatalf("third get: got %d, want 42", v)
	}
	if calls.Load() != 3 {
		t.Fatalf("build ran %d times, want 3 (panic and error not cached)", calls.Load())
	}
	if !l.resolved() {
		t.Fatal("resolved is false after success")
	}
	if _, err := l.get(build); err != nil {
		t.Fatalf("cached get: unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Fatalf("build ran %d times after success, want 3", calls.Load())
	}
}

func TestLazy_GoexitReleasesWaiters(t *testing.T) {
	var l lazy[int]
	var calls atomic.Int32
	var goexited atomic.Bool
	//nolint:unparam // get requires (T, error); Goexit path is nil by design
	build := func() (int, error) {
		if calls.Add(1) == 1 {
			goexited.Store(true)
			runtime.Goexit()
		}
		return 5, nil
	}

	// The first caller Goexits, so its get never returns and it sends no
	// result; the remaining n-1 callers must be released and succeed.
	const n = 4
	start := make(chan struct{})
	results := make(chan int, n)
	errsCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			v, err := l.get(build)
			results <- v
			errsCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errsCh)

	if !goexited.Load() {
		t.Fatal("first build did not Goexit")
	}
	if calls.Load() != 2 {
		t.Fatalf("build ran %d times, want 2 (Goexit not cached, one retry)", calls.Load())
	}
	if got := len(results); got != n-1 {
		t.Fatalf("got %d results, want %d (Goexiter sends none)", got, n-1)
	}
	for v := range results {
		var buildErr BuildPanicError
		if errors.As(<-errsCh, &buildErr) {
			t.Errorf("Goexit produced BuildPanicError %v, want plain retry", buildErr)
		}
		if v != 5 {
			t.Errorf("waiter got %d, want 5", v)
		}
	}
	for err := range errsCh {
		t.Errorf("unexpected error: %v", err)
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

// TestGetCtx_PropagatesValueIntoBuilder pins that a caller's context values
// reach the builder function, so cancellation and tracing propagate into
// service construction instead of being dropped.
func TestGetCtx_PropagatesValueIntoBuilder(t *testing.T) {
	var l lazy[string]

	type ctxKey string
	const key ctxKey = "k"

	ctx := context.WithValue(context.Background(), key, "trace-id-123")

	var seen string
	v, err := l.getCtx(ctx, func(bctx context.Context) (string, error) {
		seen, _ = bctx.Value(key).(string)
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("getCtx: unexpected error: %v", err)
	}
	if v != "ok" {
		t.Fatalf("getCtx = %q, want %q", v, "ok")
	}
	if seen != "trace-id-123" {
		t.Fatalf("builder saw ctx value %q, want %q", seen, "trace-id-123")
	}
}

// TestContainer_RootCtxDetachedUntilClose pins that the container's
// builder-facing root context is detached (no caller cancellation, no
// deadline) during the container's lifetime and is canceled by Close.
func TestContainer_RootCtxDetachedUntilClose(t *testing.T) {
	c := New(nil)

	if err := c.rootCtx.Err(); err != nil {
		t.Fatalf("rootCtx canceled before Close: %v", err)
	}
	if _, hasDeadline := c.rootCtx.Deadline(); hasDeadline {
		t.Fatal("rootCtx carries a deadline, want detached")
	}

	if err := c.Close(t.Context()); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := c.rootCtx.Err(); err == nil {
		t.Fatal("rootCtx not canceled after Close")
	}
}

// TestGRPCClient_BuildSeesRootCtx pins that the root context reaches the
// grpcclient builder: a canceled root ctx makes the build fail inside
// grpcclient.New, proving the ctx is threaded into service construction.
func TestGRPCClient_BuildSeesRootCtx(t *testing.T) {
	c := New(nil)
	t.Cleanup(func() { _ = c.Close(t.Context()) })

	c.cancelRoot()

	_, err := c.GRPCClient("passthrough://localhost:1", grpcclient.WithInsecure())
	if err == nil {
		t.Fatal("GRPCClient succeeded with canceled root ctx, want error")
	}
	if !strings.Contains(err.Error(), "grpcclient: context") {
		t.Fatalf("GRPCClient error %q does not mention grpcclient: context", err)
	}
}
