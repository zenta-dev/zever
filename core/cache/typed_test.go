package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/cache/memory"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/shared/codec"
)

type mapBackend struct {
	store  map[string][]byte
	getErr error
	setErr error
	opErr  error
}

func newMapBackend() *mapBackend { return &mapBackend{store: map[string][]byte{}} }

func (m *mapBackend) Get(_ context.Context, key string) ([]byte, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}

	v, ok := m.store[key]
	if !ok {
		return nil, &cache.NotFoundError{Key: key}
	}

	return v, nil
}

func (m *mapBackend) Set(_ context.Context, key string, v []byte, _ time.Duration) error {
	if m.setErr != nil {
		return m.setErr
	}

	m.store[key] = v

	return nil
}

func (m *mapBackend) SetIfAbsent(_ context.Context, key string, v []byte, _ time.Duration) (bool, error) {
	if m.opErr != nil {
		return false, m.opErr
	}

	if _, ok := m.store[key]; ok {
		return false, nil
	}

	m.store[key] = v

	return true, nil
}

func (m *mapBackend) Delete(_ context.Context, key string) error {
	if m.opErr != nil {
		return m.opErr
	}

	delete(m.store, key)

	return nil
}

func (m *mapBackend) Increment(_ context.Context, _ string) error { return m.opErr }
func (m *mapBackend) Decrement(_ context.Context, _ string) error { return m.opErr }

func (m *mapBackend) Exists(_ context.Context, key string) (bool, error) {
	if m.opErr != nil {
		return false, m.opErr
	}

	_, ok := m.store[key]

	return ok, nil
}

func (m *mapBackend) Close(_ context.Context) error { return m.opErr }

var _ cache.Cache = (*mapBackend)(nil)

type prefixCodec struct{ failEncode, failDecode bool }

func (c prefixCodec) Encode(v string) ([]byte, error) {
	if c.failEncode {
		return nil, errors.New("encode boom")
	}

	return []byte("P:" + v), nil
}

func (c prefixCodec) Decode(b []byte) (string, error) {
	if c.failDecode {
		return "", errors.New("decode boom")
	}

	s := string(b)
	if len(s) < 2 || s[:2] != "P:" {
		return "", errors.New("bad prefix")
	}

	return s[2:], nil
}

var _ codec.Codec[string] = prefixCodec{}

func TestTyped_keyTypes_mapToStrings(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b := newMapBackend()

	ts := cache.NewTyped[string, string](b, prefixCodec{})
	if err := ts.Set(ctx, "a", "x", 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if _, ok := b.store["a"]; !ok {
		t.Error("backend missing key a")
	}

	ti := cache.NewTyped[int64, string](b, prefixCodec{})
	if err := ti.Set(ctx, -7, "y", 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if _, ok := b.store["-7"]; !ok {
		t.Error("backend missing key -7")
	}

	tf := cache.NewTyped[float64, string](b, prefixCodec{})
	if err := tf.Set(ctx, 1.5, "z", 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if _, ok := b.store["1.5"]; !ok {
		t.Error("backend missing key 1.5")
	}
}

func TestTyped_roundTrip_memoryBackend(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	backend, err := memory.New(cache.Options{})
	if err != nil {
		t.Fatalf("memory.New() error = %v", err)
	}

	defer backend.Close(ctx)

	tc := cache.NewTyped[string, string](backend, prefixCodec{})

	if setErr := tc.Set(ctx, "k", "v", time.Minute); setErr != nil {
		t.Fatalf("Set() error = %v", setErr)
	}

	got, err := tc.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got != "v" {
		t.Errorf("Get() = %q, want v", got)
	}

	ok, err := tc.SetIfAbsent(ctx, "k", "other", time.Minute)
	if err != nil || ok {
		t.Errorf("SetIfAbsent() = %v,%v want false,nil", ok, err)
	}

	ok, err = tc.SetIfAbsent(ctx, "new", "n", time.Minute)
	if err != nil || !ok {
		t.Errorf("SetIfAbsent() = %v,%v want true,nil", ok, err)
	}

	exists, err := tc.Exists(ctx, "k")
	if err != nil || !exists {
		t.Errorf("Exists() = %v,%v want true,nil", exists, err)
	}

	if err := tc.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := tc.Get(ctx, "k"); !errors.Is(err, cache.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
	}

	if err := tc.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestTyped_errors_wrapped(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("backend get notfound preserved", func(t *testing.T) {
		t.Parallel()

		tc := cache.NewTyped[string, string](newMapBackend(), prefixCodec{})
		if _, err := tc.Get(ctx, "missing"); !errors.Is(err, cache.ErrNotFound) {
			t.Errorf("errors.Is(err, ErrNotFound) = false (err = %v)", err)
		}
	})

	t.Run("decode failure wrapped", func(t *testing.T) {
		t.Parallel()

		b := newMapBackend()
		b.store["k"] = []byte("no-prefix")
		tc := cache.NewTyped[string, string](b, prefixCodec{})

		if _, err := tc.Get(ctx, "k"); err == nil {
			t.Error("Get() = nil, want decode error")
		}
	})

	t.Run("encode failure blocks set", func(t *testing.T) {
		t.Parallel()

		tc := cache.NewTyped[string, string](newMapBackend(), prefixCodec{failEncode: true})
		if err := tc.Set(ctx, "k", "v", 0); err == nil {
			t.Error("Set() = nil, want encode error")
		}

		if _, err := tc.SetIfAbsent(ctx, "k", "v", 0); err == nil {
			t.Error("SetIfAbsent() = nil, want encode error")
		}
	})

	t.Run("backend set error wrapped", func(t *testing.T) {
		t.Parallel()

		sentinel := errors.New("db down")
		b := newMapBackend()
		b.setErr = sentinel
		tc := cache.NewTyped[string, string](b, prefixCodec{})

		if err := tc.Set(ctx, "k", "v", 0); !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
		}
	})

	t.Run("backend get error wrapped", func(t *testing.T) {
		t.Parallel()

		sentinel := errors.New("db down")
		b := newMapBackend()
		b.getErr = sentinel
		tc := cache.NewTyped[string, string](b, prefixCodec{})

		if _, err := tc.Get(ctx, "k"); !errors.Is(err, sentinel) {
			t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
		}
	})

	t.Run("counter passthrough delegates to backend", func(t *testing.T) {
		t.Parallel()

		tc := cache.NewTyped[string, string](newMapBackend(), prefixCodec{})

		if err := tc.Increment(ctx, "c"); err != nil {
			t.Errorf("Increment() error = %v", err)
		}

		if err := tc.Decrement(ctx, "c"); err != nil {
			t.Errorf("Decrement() error = %v", err)
		}

		if err := tc.Delete(ctx, "c"); err != nil {
			t.Errorf("Delete() error = %v", err)
		}
	})

	t.Run("backend op errors wrapped", func(t *testing.T) {
		t.Parallel()

		sentinel := errors.New("db down")
		b := newMapBackend()
		b.opErr = sentinel
		tc := cache.NewTyped[string, string](b, prefixCodec{})

		if _, err := tc.SetIfAbsent(ctx, "k", "v", 0); !errors.Is(err, sentinel) {
			t.Errorf("SetIfAbsent errors.Is(err, sentinel) = false (err = %v)", err)
		}

		if err := tc.Delete(ctx, "k"); !errors.Is(err, sentinel) {
			t.Errorf("Delete errors.Is(err, sentinel) = false (err = %v)", err)
		}

		if err := tc.Increment(ctx, "k"); !errors.Is(err, sentinel) {
			t.Errorf("Increment errors.Is(err, sentinel) = false (err = %v)", err)
		}

		if err := tc.Decrement(ctx, "k"); !errors.Is(err, sentinel) {
			t.Errorf("Decrement errors.Is(err, sentinel) = false (err = %v)", err)
		}

		if _, err := tc.Exists(ctx, "k"); !errors.Is(err, sentinel) {
			t.Errorf("Exists errors.Is(err, sentinel) = false (err = %v)", err)
		}

		if err := tc.Close(ctx); !errors.Is(err, sentinel) {
			t.Errorf("Close errors.Is(err, sentinel) = false (err = %v)", err)
		}
	})
}
