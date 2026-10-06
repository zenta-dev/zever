package cache_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/cache"
	coredb "github.com/zenta-dev/zever/core/db"
)

func TestCacheCover_RegisterSharedDuplicate(t *testing.T) {
	t.Parallel()

	a := freshCacheAdapter()
	stub := func(coredb.DB, cache.Options) (cache.Cache, error) { return stubCache{}, nil }

	if err := cache.RegisterShared(a, stub); err != nil {
		t.Fatalf("RegisterShared() error = %v", err)
	}

	err := cache.RegisterShared(a, stub)
	if !errors.Is(err, cache.ErrDuplicate) {
		t.Fatalf("RegisterShared(dup) err = %v, want ErrDuplicate", err)
	}

	var dupErr cache.DuplicateError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(err, DuplicateError) = false (err = %T %v)", err, err)
	}

	if dupErr.Adapter != a {
		t.Errorf("DuplicateError.Adapter = %v, want %v", dupErr.Adapter, a)
	}
}

// customStringKey is a named string kind. Type-switch cases on exact
// builtin types miss it, so Typed.key must fall through to the fmt.Sprint
// default branch.
type customStringKey string

func TestCacheCover_TypedCustomKeyDefaultBranch(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	b := newMapBackend()
	tc := cache.NewTyped[customStringKey, string](b, prefixCodec{})

	if err := tc.Set(ctx, customStringKey("abc"), "v", 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if _, ok := b.store["abc"]; !ok {
		t.Fatalf("backend missing key %q (store = %v)", "abc", b.store)
	}

	got, err := tc.Get(ctx, "abc")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got != "v" {
		t.Errorf("Get() = %q, want v", got)
	}
}
