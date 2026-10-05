package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/shared/lrucache"
)

// defaultMaxEntries bounds the lease table before LRU eviction, matching the
// cache/memory adapter default. It applies when lock.Options.MaxEntries is
// non-positive.
const defaultMaxEntries = 1000

// adapter is an in-process lock.Locker over lrucache.TTLCache. The library
// owns storage, TTL expiry, and all check-and-mutate atomicity; the adapter
// owns key prefixing, TTL/RetryInterval defaults, and holder-id generation.
// It is safe for concurrent use: single ops hold mu as readers while calling
// into the library (which serializes on its own lock) so Close can swap the
// table out from under them.
//
// Expiry boundary is unchanged from the previous hand-rolled map: a lease is
// live while now is strictly before its deadline and expired at the exact
// deadline and after.
type adapter struct {
	mu            sync.RWMutex
	leases        *lrucache.TTLCache[string, string]
	prefix        string
	ttl           time.Duration
	retryInterval time.Duration
	maxEntries    int
}

// handle is one acquired lock.Lock.
type handle struct {
	a      *adapter
	key    string
	holder string
}

// New creates an in-process lock.Locker. Non-positive TTL and retry
// intervals fall back to lock.DefaultTTL and lock.DefaultRetryInterval.
// Non-positive MaxEntries falls back to 1000, matching the cache/memory
// adapter: past the bound the least-recently-used live lease is evicted
// and its holder observes lock.ErrNotHeld.
func New(opts lock.Options) (lock.Locker, error) {
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = lock.DefaultTTL
	}

	retry := opts.RetryInterval
	if retry <= 0 {
		retry = lock.DefaultRetryInterval
	}

	maxEntries := opts.MaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultMaxEntries
	}

	return &adapter{
		leases:        lrucache.NewTTL[string, string](maxEntries, 0),
		prefix:        opts.Prefix,
		ttl:           ttl,
		retryInterval: retry,
		maxEntries:    maxEntries,
	}, nil
}

// randRead is a seam for newHolderID, stubbed in tests to force failure.
var randRead = rand.Read

// newHolderID returns a random hex holder id.
func newHolderID() (string, error) {
	var b [16]byte

	if _, err := randRead(b[:]); err != nil {
		return "", fmt.Errorf("lock: holder id: %w", err)
	}

	return hex.EncodeToString(b[:]), nil
}

// equalHolder reports whether two lease holders are the same.
func equalHolder(a, b string) bool { return a == b }

// TryAcquire attempts to acquire key exactly once, reporting ok=false with
// a nil error when the key is currently held. A non-positive ttl selects
// the adapter default. An expired lease counts as absent, so a lapsed holder
// never blocks a successor.
func (a *adapter) TryAcquire(_ context.Context, key string, ttl time.Duration) (lock.Lock, bool, error) {
	if key == "" {
		return nil, false, fmt.Errorf("lock: try acquire %q: key is empty", key)
	}

	if ttl <= 0 {
		ttl = a.ttl
	}

	holder, err := newHolderID()
	if err != nil {
		return nil, false, err
	}

	k := a.prefix + key

	a.mu.RLock()
	stored := a.leases.SetIfAbsent(k, holder, ttl)
	a.mu.RUnlock()

	if !stored {
		return nil, false, nil
	}

	return &handle{a: a, key: key, holder: holder}, true, nil
}

// Acquire blocks, retrying every retry interval, until the lock is acquired
// or ctx is done. It returns ctx.Err() wrapped if the context lapses first.
// The retry timer is created only after the first claim fails, so the
// uncontended path allocates nothing for it.
func (a *adapter) Acquire(ctx context.Context, key string, ttl time.Duration) (lock.Lock, error) {
	l, ok, err := a.TryAcquire(ctx, key, ttl)
	if err != nil {
		return nil, err
	}

	if ok {
		return l, nil
	}

	t := time.NewTimer(a.retryInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("lock: acquire %q: %w", key, ctx.Err())
		case <-t.C:
		}

		l, ok, err = a.TryAcquire(ctx, key, ttl)
		if err != nil {
			return nil, err
		}

		if ok {
			return l, nil
		}

		t.Reset(a.retryInterval)
	}
}

// Close drops every lease record. Leases still held by callers are not
// released individually; they expire on their own TTL. Close is idempotent
// and always reports nil; work issued after Close runs against a fresh
// empty table, exactly as with the previous map reset.
func (a *adapter) Close(_ context.Context) error {
	a.mu.Lock()
	a.leases = lrucache.NewTTL[string, string](a.maxEntries, 0)
	a.mu.Unlock()

	return nil
}

// Key returns the locked key.
func (h *handle) Key() string { return h.key }

// Extend renews the lease for a further ttl, or the adapter default when
// ttl is non-positive. It returns lock.ErrNotHeld when the lease was
// already lost.
func (h *handle) Extend(_ context.Context, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = h.a.ttl
	}

	k := h.a.prefix + h.key

	h.a.mu.RLock()
	extended := h.a.leases.CompareAndExtend(k, h.holder, ttl, equalHolder)
	h.a.mu.RUnlock()

	if !extended {
		return fmt.Errorf("lock: extend %q: %w", h.key, lock.ErrNotHeld)
	}

	return nil
}

// Unlock releases the lease. It returns lock.ErrNotHeld when the lease was
// already lost, and never deletes a lease it no longer owns: after the TTL
// lapses the entry may already belong to another holder.
func (h *handle) Unlock(_ context.Context) error {
	k := h.a.prefix + h.key

	h.a.mu.RLock()
	released := h.a.leases.CompareAndDelete(k, h.holder, equalHolder)
	h.a.mu.RUnlock()

	if !released {
		return fmt.Errorf("lock: unlock %q: %w", h.key, lock.ErrNotHeld)
	}

	return nil
}
