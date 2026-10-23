package cache_test

import (
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/core/cache"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()
	cases := map[string][2]string{
		"ErrNotFound":       {cache.ErrNotFound.Error(), "cache: not found"},
		"ErrClosed":         {cache.ErrClosed.Error(), "cache: closed"},
		"ErrNilFactory":     {cache.ErrNilFactory.Error(), "cache: nil factory"},
		"ErrDuplicate":      {cache.ErrDuplicate.Error(), "cache: duplicate adapter"},
		"ErrUnknownAdapter": {cache.ErrUnknownAdapter.Error(), "cache: unknown adapter"},
		"ErrInvalidAdapter": {cache.ErrInvalidAdapter.Error(), "cache: invalid adapter"},
		"ErrInvalidValue":   {cache.ErrInvalidValue.Error(), "cache: invalid value"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestMemoryNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	c, err := memory.New(cache.Options{})
	if err != nil {
		t.Fatalf("memory.New() error = %v", err)
	}
	defer c.Close(ctx)
	_, err = c.Get(ctx, "missing")
	if err == nil {
		t.Fatal("Get(missing) = nil, want ErrNotFound")
	}
	if !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
	}
	var nfErr cache.NotFoundError
	if !errors.As(err, &nfErr) {
		t.Errorf("errors.As(err, NotFoundError) = false (err = %T %v)", err, err)
	}
}

func TestMemoryClosed(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	c, err := memory.New(cache.Options{})
	if err != nil {
		t.Fatalf("memory.New() error = %v", err)
	}
	if closeErr := c.Close(ctx); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}
	err = c.Set(ctx, "k", []byte("v"), time.Second)
	if err == nil {
		t.Fatal("Set(closed) = nil, want ErrClosed")
	}
	if !errors.Is(err, cache.ErrClosed) {
		t.Errorf("errors.Is(err, ErrClosed) = false (err = %v)", err)
	}
}

func TestMemoryInvalidInteger(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	c, err := memory.New(cache.Options{})
	if err != nil {
		t.Fatalf("memory.New() error = %v", err)
	}
	defer c.Close(ctx)
	if setErr := c.Set(ctx, "counter", []byte("abc"), time.Second); setErr != nil {
		t.Fatalf("Set() error = %v", setErr)
	}
	err = c.Increment(ctx, "counter")
	if err == nil {
		t.Fatal("Increment(abc) = nil, want ErrInvalidValue")
	}
	if !errors.Is(err, cache.ErrInvalidValue) {
		t.Errorf("errors.Is(err, ErrInvalidValue) = false (err = %v)", err)
	}
}


func TestTypedErrorMessages_unwrap(t *testing.T) {
	t.Parallel()

	t.Run("notfound", func(t *testing.T) {
		t.Parallel()

		err := cache.NotFoundError{Key: "k"}
		if got, want := err.Error(), `cache: not found: key "k"`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, cache.ErrNotFound) {
			t.Error("errors.Is(err, ErrNotFound) = false")
		}
	})

	t.Run("invalid value with cause joins both", func(t *testing.T) {
		t.Parallel()

		cause := errors.New("boom")
		err := cache.InvalidValueError{Key: "k", Err: cause}
		if got, want := err.Error(), `cache: invalid value: key "k": boom`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, cache.ErrInvalidValue) {
			t.Error("errors.Is(err, ErrInvalidValue) = false")
		}

		if !errors.Is(err, cause) {
			t.Error("errors.Is(err, cause) = false")
		}
	})

	t.Run("invalid value nil cause", func(t *testing.T) {
		t.Parallel()

		err := cache.InvalidValueError{Key: "k"}
		if !errors.Is(err, cache.ErrInvalidValue) {
			t.Error("errors.Is(err, ErrInvalidValue) = false")
		}
	})

	t.Run("invalid adapter", func(t *testing.T) {
		t.Parallel()

		err := cache.InvalidAdapterError{Adapter: "bogus"}
		if got, want := err.Error(), `cache: invalid adapter: "bogus"`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, cache.ErrInvalidAdapter) {
			t.Error("errors.Is(err, ErrInvalidAdapter) = false")
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		err := cache.DuplicateAdapterError{Adapter: "memory"}
		if got, want := err.Error(), `cache: duplicate adapter: memory`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, cache.ErrDuplicate) {
			t.Error("errors.Is(err, ErrDuplicate) = false")
		}
	})

	t.Run("unknown adapter", func(t *testing.T) {
		t.Parallel()

		err := cache.UnknownAdapterError{Adapter: "unknown"}
		if got, want := err.Error(), `cache: unknown adapter: unknown (forgotten import?)`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, cache.ErrUnknownAdapter) {
			t.Error("errors.Is(err, ErrUnknownAdapter) = false")
		}
	})
}
