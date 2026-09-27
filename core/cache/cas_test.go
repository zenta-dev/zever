package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/cache"
)

type baseOnlyCache struct{ stubCache }

func TestBaseInterfaceNotWidened(t *testing.T) {
	t.Parallel()

	var _ cache.Cache = baseOnlyCache{}

	if _, ok := any(baseOnlyCache{}).(cache.CompareAndSwapCache); ok {
		t.Fatal("base-only adapter unexpectedly implements CompareAndSwapCache")
	}
}

func TestCompareAndSwapCache_shape(t *testing.T) {
	t.Parallel()

	var c cache.CompareAndSwapCache

	_ = c

	var _ interface {
		CompareAndDelete(context.Context, string, []byte) (bool, error)
		CompareAndExtend(context.Context, string, []byte, time.Duration) (bool, error)
	} = c
}
