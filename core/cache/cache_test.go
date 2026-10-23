package cache_test

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/zenta-dev/zever/core/cache"
)

var cacheAdapterSeq atomic.Int64

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
