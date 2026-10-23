package cache_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/cache"
)

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
