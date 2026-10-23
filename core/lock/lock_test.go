package lock_test

import (
	"context"
	"time"

	"github.com/zenta-dev/zever/core/lock"
)

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
