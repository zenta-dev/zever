package cache_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/core/cache"
	coredb "github.com/zenta-dev/zever/core/db"
)

func TestCacheEdge_TypedKeyConversions(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	tests := []struct {
		name string
		set  func(backend *mapBackend) error
		key  string
	}{
		{
			name: "int8",
			set:  func(b *mapBackend) error { return cache.NewTyped[int8, string](b, prefixCodec{}).Set(ctx, -7, "v", 0) },
			key:  "-7",
		},
		{
			name: "int16",
			set: func(b *mapBackend) error {
				return cache.NewTyped[int16, string](b, prefixCodec{}).Set(ctx, 300, "v", 0)
			},
			key: "300",
		},
		{
			name: "int32",
			set: func(b *mapBackend) error {
				return cache.NewTyped[int32, string](b, prefixCodec{}).Set(ctx, 70000, "v", 0)
			},
			key: "70000",
		},
		{
			name: "float32",
			set: func(b *mapBackend) error {
				return cache.NewTyped[float32, string](b, prefixCodec{}).Set(ctx, 1.5, "v", 0)
			},
			key: "1.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := newMapBackend()

			if err := tt.set(b); err != nil {
				t.Fatalf("Set() error = %v", err)
			}

			if _, ok := b.store[tt.key]; !ok {
				t.Fatalf("backend missing key %q", tt.key)
			}
		})
	}
}

func TestCacheEdge_TypedSetIfAbsentDuplicate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	tc := cache.NewTyped[string, string](newMapBackend(), prefixCodec{})

	ok, err := tc.SetIfAbsent(ctx, "k", "first", 0)
	if err != nil || !ok {
		t.Fatalf("SetIfAbsent(first) = %v,%v want true,nil", ok, err)
	}

	ok, err = tc.SetIfAbsent(ctx, "k", "second", 0)
	if err != nil || ok {
		t.Fatalf("SetIfAbsent(duplicate) = %v,%v want false,nil", ok, err)
	}
}

func TestCacheEdge_TypedZeroTTL(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	backend, err := memory.New(cache.Options{})
	if err != nil {
		t.Fatalf("memory.New() error = %v", err)
	}

	defer func() { _ = backend.Close(ctx) }()

	tc := cache.NewTyped[string, string](backend, prefixCodec{})

	if setErr := tc.Set(ctx, "k", "v", 0); setErr != nil {
		t.Fatalf("Set(ttl=0) error = %v", setErr)
	}

	got, err := tc.Get(ctx, "k")
	if err != nil || got != "v" {
		t.Fatalf("Get() = %q,%v want v,nil", got, err)
	}
}

func TestCacheEdge_ErrorsIsAs(t *testing.T) {
	t.Parallel()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		err := cache.NotFoundError{Key: "k"}
		if !errors.Is(err, cache.ErrNotFound) {
			t.Fatalf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
		}

		var nf cache.NotFoundError
		if !errors.As(err, &nf) || nf.Key != "k" {
			t.Fatalf("errors.As(err, NotFoundError) = false or Key = %q", nf.Key)
		}
	})

	t.Run("invalid value with cause", func(t *testing.T) {
		t.Parallel()

		cause := errors.New("boom")
		err := cache.InvalidValueError{Key: "k", Err: cause}

		if !errors.Is(err, cache.ErrInvalidValue) {
			t.Fatalf("errors.Is(err, ErrInvalidValue) = false (err = %v)", err)
		}

		if !errors.Is(err, cause) {
			t.Fatalf("errors.Is(err, cause) = false (err = %v)", err)
		}

		var iv cache.InvalidValueError
		if !errors.As(err, &iv) || iv.Key != "k" {
			t.Fatalf("errors.As(err, InvalidValueError) = false or Key = %q", iv.Key)
		}
	})
}

func TestCacheEdge_ConcurrentTypedMemory(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	backend, err := memory.New(cache.Options{})
	if err != nil {
		t.Fatalf("memory.New() error = %v", err)
	}

	defer func() { _ = backend.Close(ctx) }()

	tc := cache.NewTyped[string, string](backend, prefixCodec{})

	var wg sync.WaitGroup

	for i := range 50 {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			key := fmt.Sprintf("k-%d", i)
			if err := tc.Set(ctx, key, "v", time.Minute); err != nil {
				t.Errorf("Set(%q) error = %v", key, err)

				return
			}

			if _, err := tc.Get(ctx, key); err != nil {
				t.Errorf("Get(%q) error = %v", key, err)
			}
		}(i)
	}

	wg.Wait()
}

func TestCacheEdge_OptionsValidateAlwaysNil(t *testing.T) {
	t.Parallel()

	opts := cache.Options{Capacity: -1, MemoryOptions: cache.MemoryOptions{MaxEntries: -1}}
	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestCacheEdge_SharedRegistry(t *testing.T) {
	t.Parallel()

	t.Run("nil factory", func(t *testing.T) {
		t.Parallel()

		if err := cache.RegisterShared(freshCacheAdapter(), nil); !errors.Is(err, cache.ErrNilFactory) {
			t.Fatalf("RegisterShared(nil) err = %v, want ErrNilFactory", err)
		}
	})

	t.Run("unknown adapter", func(t *testing.T) {
		t.Parallel()

		if _, err := cache.OpenShared(freshCacheAdapter(), nil, cache.Options{}); !errors.Is(err, cache.ErrUnknownAdapter) {
			t.Fatalf("OpenShared unknown err = %v, want ErrUnknownAdapter", err)
		}
	})

	t.Run("factory error wrapped", func(t *testing.T) {
		t.Parallel()

		adapter := freshCacheAdapter()
		sentinel := errors.New("boom")

		if err := cache.RegisterShared(adapter, func(coredb.DB, cache.Options) (cache.Cache, error) {
			return nil, sentinel
		}); err != nil {
			t.Fatalf("RegisterShared err = %v", err)
		}

		if _, err := cache.OpenShared(adapter, nil, cache.Options{}); !errors.Is(err, sentinel) {
			t.Fatalf("OpenShared err = %v, want wrapped sentinel", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		adapter := freshCacheAdapter()
		want := stubCache{}

		if err := cache.RegisterShared(adapter, func(coredb.DB, cache.Options) (cache.Cache, error) {
			return want, nil
		}); err != nil {
			t.Fatalf("RegisterShared err = %v", err)
		}

		got, err := cache.OpenShared(adapter, nil, cache.Options{})
		if err != nil {
			t.Fatalf("OpenShared err = %v", err)
		}

		if got != cache.Cache(want) {
			t.Fatal("OpenShared did not return the factory cache")
		}
	})
}
