package memory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/zenta-dev/zever/lock"
)

// lease is one held key: who holds it and when the lease lapses.
type lease struct {
	holder  string
	expires time.Time
}

// adapter is an in-process lock.Locker. It is safe for concurrent use.
type adapter struct {
	mu            sync.Mutex
	leases        map[string]*lease
	prefix        string
	ttl           time.Duration
	retryInterval time.Duration
}

// handle is one acquired lock.Lock.
type handle struct {
	a      *adapter
	key    string
	holder string
}

// New creates an in-process lock.Locker. Non-positive TTL and retry
// intervals fall back to lock.DefaultTTL and lock.DefaultRetryInterval.
func New(opts lock.Options) (lock.Locker, error) {
	ttl := opts.TTL
	if ttl <= 0 {
		ttl = lock.DefaultTTL
	}

	retry := opts.RetryInterval
	if retry <= 0 {
		retry = lock.DefaultRetryInterval
	}

	return &adapter{
		leases:        make(map[string]*lease),
		prefix:        opts.Prefix,
		ttl:           ttl,
		retryInterval: retry,
	}, nil
}

// randRead is a seam for newHolderID, stubbed in tests to force failure.
var randRead = rand.Read

// newHolderID returns a random hex holder id.
func newHolderID() (string, error) {
	var b [16]byte

	if _, err := randRead(b[:]); err != nil {
		return "", fmt.Errorf("[lock] holder id error: %w", err)
	}

	return hex.EncodeToString(b[:]), nil
}

// TryAcquire attempts to acquire key exactly once, reporting ok=false with
// a nil error when the key is currently held. A non-positive ttl selects
// the adapter default.
func (a *adapter) TryAcquire(_ context.Context, key string, ttl time.Duration) (lock.Lock, bool, error) {
	if key == "" {
		return nil, false, fmt.Errorf("[lock] TryAcquire %q error: key is empty", key)
	}

	if ttl <= 0 {
		ttl = a.ttl
	}

	holder, err := newHolderID()
	if err != nil {
		return nil, false, err
	}

	now := time.Now()
	k := a.prefix + key

	a.mu.Lock()
	defer a.mu.Unlock()

	if l, ok := a.leases[k]; ok && now.Before(l.expires) {
		return nil, false, nil
	}

	a.leases[k] = &lease{holder: holder, expires: now.Add(ttl)}

	return &handle{a: a, key: key, holder: holder}, true, nil
}

// Acquire blocks, retrying every retry interval, until the lock is acquired
// or ctx is done. It returns ctx.Err() wrapped if the context lapses first.
func (a *adapter) Acquire(ctx context.Context, key string, ttl time.Duration) (lock.Lock, error) {
	t := time.NewTimer(a.retryInterval)
	defer t.Stop()

	for {
		l, ok, err := a.TryAcquire(ctx, key, ttl)
		if err != nil {
			return nil, err
		}

		if ok {
			return l, nil
		}

		t.Reset(a.retryInterval)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("[lock] Acquire %q error: %w", key, ctx.Err())
		case <-t.C:
		}
	}
}

// Close drops every lease record. Leases still held by callers are not
// released individually; they expire on their own TTL.
func (a *adapter) Close(_ context.Context) error {
	a.mu.Lock()
	a.leases = make(map[string]*lease)
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

	now := time.Now()
	k := h.a.prefix + h.key

	h.a.mu.Lock()
	defer h.a.mu.Unlock()

	l, ok := h.a.leases[k]
	if !ok || l.holder != h.holder || !now.Before(l.expires) {
		return fmt.Errorf("[lock] Extend %q error: %w", h.key, lock.ErrNotHeld)
	}

	l.expires = now.Add(ttl)

	return nil
}

// Unlock releases the lease. It returns lock.ErrNotHeld when the lease was
// already lost, and never deletes a lease it no longer owns: after the TTL
// lapses the entry may already belong to another holder.
func (h *handle) Unlock(_ context.Context) error {
	now := time.Now()
	k := h.a.prefix + h.key

	h.a.mu.Lock()
	defer h.a.mu.Unlock()

	l, ok := h.a.leases[k]
	if !ok || l.holder != h.holder || !now.Before(l.expires) {
		return fmt.Errorf("[lock] Unlock %q error: %w", h.key, lock.ErrNotHeld)
	}

	delete(h.a.leases, k)

	return nil
}
