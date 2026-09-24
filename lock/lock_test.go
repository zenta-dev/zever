package lock_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/lock"
	"github.com/zenta-dev/zever/lock/memory"
)

var lockAdapterSeq atomic.Int64

func freshLockAdapter() lock.Adapter { return lock.Adapter(1000 + lockAdapterSeq.Add(1)) }

type stubLock struct{ key string }

func (s stubLock) Key() string                                 { return s.key }
func (s stubLock) Extend(context.Context, time.Duration) error { return nil }
func (s stubLock) Unlock(context.Context) error                { return nil }

type stubLocker struct{}

func (stubLocker) TryAcquire(_ context.Context, key string, _ time.Duration) (lock.Lock, bool, error) {
	return stubLock{key: key}, true, nil
}

func (stubLocker) Acquire(_ context.Context, key string, _ time.Duration) (lock.Lock, error) {
	return stubLock{key: key}, nil
}

func (stubLocker) Close(context.Context) error { return nil }

var _ lock.Locker = stubLocker{}
var _ lock.Lock = stubLock{}

func TestLockOpen_registeredFactory_returnsLocker(t *testing.T) {
	a := freshLockAdapter()
	wantOpts := lock.Options{Prefix: "t:"}
	var gotOpts lock.Options

	if err := lock.Register(a, func(opts lock.Options) (lock.Locker, error) {
		gotOpts = opts
		return stubLocker{}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := lock.Open(a, wantOpts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if got == nil {
		t.Fatal("Open() = nil, want locker")
	}

	if gotOpts != wantOpts {
		t.Errorf("factory opts = %+v, want %+v", gotOpts, wantOpts)
	}

	l, ok, err := got.TryAcquire(t.Context(), "k", time.Minute)
	if err != nil || !ok || l == nil {
		t.Fatalf("TryAcquire = (%v, %v, %v), want (lock, true, nil)", l, ok, err)
	}
}

func TestLockRegister_nilFactory_returnsNilFactory(t *testing.T) {
	err := lock.Register(freshLockAdapter(), nil)
	if err == nil {
		t.Fatal("Register(nil) = nil, want ErrNilFactory")
	}

	if !errors.Is(err, lock.ErrNilFactory) {
		t.Errorf("errors.Is(err, ErrNilFactory) = false (err = %v)", err)
	}
}

func TestLockRegister_duplicate_returnsDuplicate(t *testing.T) {
	a := freshLockAdapter()
	stub := func(lock.Options) (lock.Locker, error) { return stubLocker{}, nil }

	if err := lock.Register(a, stub); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err := lock.Register(a, stub)
	if err == nil {
		t.Fatal("Register(dup) = nil, want ErrDuplicate")
	}

	if !errors.Is(err, lock.ErrDuplicate) {
		t.Errorf("errors.Is(err, ErrDuplicate) = false (err = %v)", err)
	}

	var dupErr *lock.DuplicateError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(err, DuplicateError) = false (err = %T %v)", err, err)
	}

	if dupErr.Adapter != a {
		t.Errorf("DuplicateError.Adapter = %v, want %v", dupErr.Adapter, a)
	}
}

func TestLockOpen_unknown_returnsUnknownAdapter(t *testing.T) {
	_, err := lock.Open(lock.Adapter(9999), lock.Options{})
	if err == nil {
		t.Fatal("Open() = nil, want ErrUnknownAdapter")
	}

	if !errors.Is(err, lock.ErrUnknownAdapter) {
		t.Errorf("errors.Is(err, ErrUnknownAdapter) = false (err = %v)", err)
	}

	var unkErr *lock.UnknownAdapterError
	if !errors.As(err, &unkErr) {
		t.Fatalf("errors.As(err, UnknownAdapterError) = false (err = %T %v)", err, err)
	}
}

func TestLockOpen_factoryError_wrapped(t *testing.T) {
	a := freshLockAdapter()
	sentinel := errors.New("boom")
	_ = lock.Register(a, func(lock.Options) (lock.Locker, error) { return nil, sentinel })

	_, err := lock.Open(a, lock.Options{})
	if err == nil {
		t.Fatal("Open() = nil, want wrapped error")
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
	}
}

func TestLockOpen_concurrentReads_safe(t *testing.T) {
	a := freshLockAdapter()
	_ = lock.Register(a, func(lock.Options) (lock.Locker, error) { return stubLocker{}, nil })

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := lock.Open(a, lock.Options{}); err != nil {
				t.Errorf("Open() error = %v", err)
			}
		}()
	}

	wg.Wait()
}

func TestLockOpen_memoryAdapter_endToEnd(t *testing.T) {
	t.Parallel()

	a := freshLockAdapter()

	if err := lock.Register(a, memory.New); err != nil {
		t.Fatalf("Register(memory) error = %v", err)
	}

	l, err := lock.Open(a, lock.Options{RetryInterval: time.Millisecond})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	ctx := t.Context()
	defer func() { _ = l.Close(ctx) }()

	held, ok, err := l.TryAcquire(ctx, "e2e", time.Minute)
	if err != nil || !ok {
		t.Fatalf("TryAcquire = (%v, %v), want (true, nil)", ok, err)
	}

	if held.Key() != "e2e" {
		t.Fatalf("Key() = %q, want e2e", held.Key())
	}

	if err := held.Extend(ctx, time.Minute); err != nil {
		t.Fatalf("Extend() error = %v", err)
	}

	if err := held.Unlock(ctx); err != nil {
		t.Fatalf("Unlock() error = %v", err)
	}

	if err := held.Unlock(ctx); !errors.Is(err, lock.ErrNotHeld) {
		t.Fatalf("second Unlock() = %v, want ErrNotHeld", err)
	}
}
