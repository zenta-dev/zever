package cache_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/cache"
)

var cacheAdapterSeq atomic.Int64

func freshCacheAdapter() cache.Adapter { return cache.Adapter(1000 + cacheAdapterSeq.Add(1)) }

type stubCache struct{}

func (stubCache) Get(context.Context, string) ([]byte, error) { return []byte("v"), nil }
func (stubCache) Set(context.Context, string, []byte, time.Duration) error {
	return nil
}
func (stubCache) SetIfAbsent(context.Context, string, []byte, time.Duration) (bool, error) {
	return true, nil
}
func (stubCache) Delete(context.Context, string) error    { return nil }
func (stubCache) Increment(context.Context, string) error { return nil }
func (stubCache) Decrement(context.Context, string) error { return nil }
func (stubCache) Exists(context.Context, string) (bool, error) {
	return true, nil
}
func (stubCache) Close(context.Context) error { return nil }

var _ cache.Cache = stubCache{}

func TestCacheOpen_registeredFactory_returnsCache(t *testing.T) {
	a := freshCacheAdapter()
	wantOpts := cache.Options{Capacity: 7}
	var gotOpts cache.Options

	if err := cache.Register(a, func(opts cache.Options) (cache.Cache, error) {
		gotOpts = opts
		return stubCache{}, nil
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, err := cache.Open(a, wantOpts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if got == nil {
		t.Fatal("Open() = nil, want cache")
	}

	if gotOpts != wantOpts {
		t.Errorf("factory opts = %+v, want %+v", gotOpts, wantOpts)
	}
}

func TestCacheRegister_nilFactory_returnsNilFactory(t *testing.T) {
	err := cache.Register(freshCacheAdapter(), nil)
	if err == nil {
		t.Fatal("Register(nil) = nil, want ErrNilFactory")
	}

	if !errors.Is(err, cache.ErrNilFactory) {
		t.Errorf("errors.Is(err, ErrNilFactory) = false (err = %v)", err)
	}
}

func TestCacheRegister_duplicate_returnsDuplicate(t *testing.T) {
	a := freshCacheAdapter()
	stub := func(cache.Options) (cache.Cache, error) { return stubCache{}, nil }

	if err := cache.Register(a, stub); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err := cache.Register(a, stub)
	if err == nil {
		t.Fatal("Register(dup) = nil, want ErrDuplicate")
	}

	if !errors.Is(err, cache.ErrDuplicate) {
		t.Errorf("errors.Is(err, ErrDuplicate) = false (err = %v)", err)
	}

	var dupErr *cache.DuplicateError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(err, DuplicateError) = false (err = %T %v)", err, err)
	}

	if dupErr.Adapter != a {
		t.Errorf("DuplicateError.Adapter = %v, want %v", dupErr.Adapter, a)
	}
}

func TestCacheOpen_unknown_returnsUnknownAdapter(t *testing.T) {
	_, err := cache.Open(cache.Adapter(9999), cache.Options{})
	if err == nil {
		t.Fatal("Open() = nil, want ErrUnknownAdapter")
	}

	if !errors.Is(err, cache.ErrUnknownAdapter) {
		t.Errorf("errors.Is(err, ErrUnknownAdapter) = false (err = %v)", err)
	}

	var unkErr *cache.UnknownAdapterError
	if !errors.As(err, &unkErr) {
		t.Fatalf("errors.As(err, UnknownAdapterError) = false (err = %T %v)", err, err)
	}
}

func TestCacheOpen_factoryError_wrapped(t *testing.T) {
	a := freshCacheAdapter()
	sentinel := errors.New("boom")
	_ = cache.Register(a, func(cache.Options) (cache.Cache, error) { return nil, sentinel })

	_, err := cache.Open(a, cache.Options{})
	if err == nil {
		t.Fatal("Open() = nil, want wrapped error")
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
	}
}

func TestCacheOpen_concurrentReads_safe(t *testing.T) {
	a := freshCacheAdapter()
	_ = cache.Register(a, func(cache.Options) (cache.Cache, error) { return stubCache{}, nil })

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := cache.Open(a, cache.Options{}); err != nil {
				t.Errorf("Open() error = %v", err)
			}
		}()
	}

	wg.Wait()
}
