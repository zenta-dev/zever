package memory

import (
	"bytes"
	"context"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/shared/lrucache"
)

const defaultMaxEntries = 1000

// DefaultSweepInterval controls how often the janitor purges expired entries when unconfigured.
const DefaultSweepInterval = time.Minute

var _ cache.CompareAndSwapCache = (*memoryAdapter)(nil)

// memoryAdapter is a cache.Cache over lrucache.TTLCache. The library owns
// storage, TTL expiry, LRU eviction, and all check-and-mutate atomicity; the
// adapter owns []byte copy semantics (the library never clones V), the closed
// flag with cache.ErrClosed (the library has no closed state), and the
// PurgeExpired janitor. It is goroutine-safe: single ops hold mu while
// calling into the library so Close excludes them, and addDelta retries
// GetWithExpiry/CompareAndSwap per iteration.
//
// Expiry boundary: the library treats an entry as expired at its exact
// deadline (!expiresAt.After(now)), while the previous hand-rolled map
// treated it as live until strictly after (now.After(expiresAt)). A read
// landing exactly on the deadline nanosecond now misses; every other
// behavior is unchanged.
type memoryAdapter struct {
	mu     sync.RWMutex
	tc     *lrucache.TTLCache[string, []byte]
	stop   chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
	closed bool
}

// New creates in-memory cache adapter using sweep interval default 1m and max entries default 1000, starts janitor.
func New(opts cache.Options) (cache.Cache, error) {
	interval := opts.SweepInterval
	if interval <= 0 {
		interval = DefaultSweepInterval
	}

	maxEntries := opts.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultMaxEntries
	}

	a := &memoryAdapter{
		tc:   lrucache.NewTTL[string, []byte](maxEntries, 0),
		stop: make(chan struct{}),
	}

	a.startJanitor(interval)

	return a, nil
}

func (a *memoryAdapter) Get(_ context.Context, key string) ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return nil, cache.ErrClosed
	}

	v, ok := a.tc.Get(key)
	if !ok {
		return nil, &cache.NotFoundError{Key: key}
	}

	return append([]byte(nil), v...), nil
}

func (a *memoryAdapter) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return cache.ErrClosed
	}

	a.tc.PutTTL(key, append([]byte(nil), value...), ttl)

	return nil
}

func (a *memoryAdapter) SetIfAbsent(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return false, cache.ErrClosed
	}

	return a.tc.SetIfAbsent(key, append([]byte(nil), value...), ttl), nil
}

func (a *memoryAdapter) Delete(_ context.Context, key string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return cache.ErrClosed
	}

	a.tc.Delete(key)

	return nil
}

// CompareAndDelete removes key only when its live value equals expected.
// Missing, expired, or mismatched entries report deleted=false with nil
// error, so a stale holder never steals a successor entry.
func (a *memoryAdapter) CompareAndDelete(_ context.Context, key string, expected []byte) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return false, cache.ErrClosed
	}

	return a.tc.CompareAndDelete(key, expected, bytes.Equal), nil
}

// CompareAndExtend renews the TTL on key only when its live value equals
// expected. A non-positive ttl clears the expiry. Missing, expired, or
// mismatched entries report extended=false with nil error.
func (a *memoryAdapter) CompareAndExtend(_ context.Context, key string, expected []byte, ttl time.Duration) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return false, cache.ErrClosed
	}

	return a.tc.CompareAndExtend(key, expected, ttl, bytes.Equal), nil
}

func (a *memoryAdapter) Increment(_ context.Context, key string) error {
	return a.addDelta(key, 1)
}

func (a *memoryAdapter) Decrement(_ context.Context, key string) error {
	return a.addDelta(key, -1)
}

// addDelta parses the live integer under key, adds delta, and swaps the
// encoding back, preserving the entry's expiry. A missing or expired key
// restarts from delta as a persist entry; a non-integer value reports
// cache.InvalidValueError. Each read-modify-write step is one library atomic;
// the loop retries on a lost race, so concurrent increments never drop an
// update.
func (a *memoryAdapter) addDelta(key string, delta int64) error {
	for {
		a.mu.RLock()

		if a.closed {
			a.mu.RUnlock()

			return cache.ErrClosed
		}

		cur, _, ok := a.tc.GetWithExpiry(key)
		if !ok {
			next := strconv.AppendInt(nil, delta, 10)
			stored := a.tc.SetIfAbsent(key, next, 0)
			a.mu.RUnlock()

			if stored {
				return nil
			}

			continue
		}

		a.mu.RUnlock()

		n, err := strconv.ParseInt(string(cur), 10, 64)
		if err != nil {
			return &cache.InvalidValueError{Key: key, Err: err}
		}

		next := strconv.AppendInt(nil, n+delta, 10)

		a.mu.RLock()

		if a.closed {
			a.mu.RUnlock()

			return cache.ErrClosed
		}

		swapped := a.tc.CompareAndSwap(key, cur, next, bytes.Equal)
		a.mu.RUnlock()

		if swapped {
			return nil
		}
	}
}

func (a *memoryAdapter) Exists(_ context.Context, key string) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.closed {
		return false, cache.ErrClosed
	}

	_, ok := a.tc.Get(key)

	return ok, nil
}

// sweep purges expired entries. The janitor calls this on a ticker; tests
// call it directly to exercise the purge path without waiting.
func (a *memoryAdapter) sweep() {
	a.tc.PurgeExpired()
}

func (a *memoryAdapter) Close(_ context.Context) error {
	a.once.Do(func() {
		a.mu.Lock()
		if a.closed {
			a.mu.Unlock()

			return
		}

		a.closed = true
		a.mu.Unlock()

		close(a.stop)
		a.wg.Wait()
	})

	return nil
}

func (a *memoryAdapter) startJanitor(interval time.Duration) {
	a.wg.Go(func() {
		jitter := time.Duration(rand.Int64N(int64(interval/10) + 1)) //nolint:gosec // weak rand is fine for jitter
		if jitter > 0 {
			interval += jitter
		}

		t := time.NewTicker(interval)
		defer t.Stop()

		for {
			select {
			case <-t.C:
				a.sweep()
			case <-a.stop:
				return
			}
		}
	})
}
