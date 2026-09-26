package job

import (
	"context"
	"time"

	"github.com/zenta-dev/zever/core/cache"
)

// UniqueLocker deduplicates jobs using cache-backed keys under a fixed prefix.
// A zero UniqueLocker is not usable without NewUniqueLocker.
type UniqueLocker struct {
	cache cache.Cache
}

const uniqueKeyPrefix = "job:unique:"

// NewUniqueLocker creates a dedup locker backed by c, for use as a
// Dispatcher.UniqueLocker or Scheduler.Locker.
func NewUniqueLocker(c cache.Cache) *UniqueLocker {
	return &UniqueLocker{cache: c}
}

func (l *UniqueLocker) uniqueKey(key string) string {
	return uniqueKeyPrefix + key
}

// Acquire claims key for ttl via SetIfAbsent with the unique key prefix.
// It requires positive ttl and returns ErrInvalidLockTTL otherwise.
func (l *UniqueLocker) Acquire(
	ctx context.Context,
	key string,
	ttl time.Duration,
) (bool, error) {
	if ttl <= 0 {
		return false, ErrInvalidLockTTL
	}

	return l.cache.SetIfAbsent(ctx, l.uniqueKey(key), []byte("1"), ttl)
}

// Release frees key acquired by Acquire.
// It deletes the prefixed cache entry.
func (l *UniqueLocker) Release(ctx context.Context, key string) error {
	return l.cache.Delete(ctx, l.uniqueKey(key))
}
