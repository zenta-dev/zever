package lrucache

import "time"

// ttlValue wraps a stored value with its absolute expiry time.
type ttlValue[V any] struct {
	val       V
	expiresAt time.Time
}

// TTLCache is a Cache wrapper that additionally expires entries after a
// fixed time-to-live. It is a separate type composing a *Cache internally
// (rather than an option baked into Cache) because TTL expiry is an
// orthogonal concern from LRU eviction: a plain Cache never needs to know
// about time, and a TTLCache always needs both an LRU capacity bound and a
// clock.
//
// Expiry is checked lazily, on access, exactly like the LRU+TTL cache in
// i18n/remote/remote.go that this type is designed to eventually replace:
// there is no background goroutine or ticker sweeping expired entries. A
// stale entry keeps occupying a capacity slot (and counts toward Len) until
// the next Get (or Delete) observes it past expiry and removes it, or until
// it is evicted by capacity pressure like any other entry.
type TTLCache[K comparable, V any] struct {
	ttl time.Duration
	c   *Cache[K, ttlValue[V]]
}

// NewTTL creates a TTLCache with the given capacity and time-to-live.
// capacity <= 0 gets the same default-capacity treatment as New. Options are
// expressed in terms of the caller-visible value type V; a WithOnEvict
// callback registered here still receives the unwrapped V (not the internal
// ttlValue[V]) and, like the base Cache, fires only on capacity eviction,
// never on an explicit Delete.
func NewTTL[K comparable, V any](capacity int, ttl time.Duration, opts ...Option[K, V]) *TTLCache[K, V] {
	// Options only ever set onEvict, so applying them to a bare (unused for
	// storage) Cache[K, V] is a cheap way to extract the caller's callback
	// and re-wrap it for the internal Cache[K, ttlValue[V]].
	tmp := &Cache[K, V]{}
	for _, opt := range opts {
		opt(tmp)
	}

	var innerOpts []Option[K, ttlValue[V]]
	if tmp.onEvict != nil {
		userOnEvict := tmp.onEvict
		innerOpts = append(innerOpts, WithOnEvict[K, ttlValue[V]](func(k K, tv ttlValue[V]) {
			userOnEvict(k, tv.val)
		}))
	}

	return &TTLCache[K, V]{
		ttl: ttl,
		c:   New[K, ttlValue[V]](capacity, innerOpts...),
	}
}

// expired reports whether tv's expiry is at or before now, matching
// i18n/remote/remote.go's `!e.expiresAt.After(now)` check.
func expired[V any](tv ttlValue[V], now time.Time) bool {
	return !tv.expiresAt.After(now)
}

// Get returns the value stored for k, moving it to the front of the
// recency list. If k is absent, or present but past its TTL, it returns
// (zero, false); an expired entry found this way is removed from the cache
// as a side effect (lazy expiry).
func (t *TTLCache[K, V]) Get(k K) (V, bool) {
	tv, ok := t.c.Get(k)
	if !ok {
		var zero V
		return zero, false
	}

	if expired(tv, time.Now()) {
		t.c.Delete(k)
		var zero V
		return zero, false
	}

	return tv.val, true
}

// Put inserts or updates the value for k, resetting its expiry to now+ttl,
// and marks it most-recently-used. If the cache is over capacity afterward,
// the least-recently-used entry is evicted (see Cache.Put); the OnEvict
// callback, if registered, fires with the evicted entry's unwrapped value.
func (t *TTLCache[K, V]) Put(k K, v V) {
	t.c.Put(k, ttlValue[V]{val: v, expiresAt: time.Now().Add(t.ttl)})
}

// Delete removes k from the cache and returns its value, if present and not
// already past its TTL. As with Get, an entry found to be expired is
// removed but reported as absent (zero, false). Like Cache.Delete, this
// never invokes the OnEvict callback.
func (t *TTLCache[K, V]) Delete(k K) (V, bool) {
	tv, ok := t.c.Delete(k)
	if !ok {
		var zero V
		return zero, false
	}

	if expired(tv, time.Now()) {
		var zero V
		return zero, false
	}

	return tv.val, true
}

// Len returns the number of entries currently in the cache, including any
// that have expired but have not yet been lazily purged by a Get or Delete.
func (t *TTLCache[K, V]) Len() int {
	return t.c.Len()
}
