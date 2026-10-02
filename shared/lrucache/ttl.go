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
	// now returns the current time. Defaults to time.Now; tests in the
	// same package may replace it with a fake clock to advance time
	// deterministically instead of sleeping. Kept unexported so NewTTL's
	// signature stays unchanged and no clock leaks into the public API.
	now func() time.Time
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
		now: time.Now,
	}
}

// nowTime returns t.now(), falling back to time.Now if the field is nil
// (e.g. a zero-value TTLCache built without NewTTL).
func (t *TTLCache[K, V]) nowTime() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// expired reports whether tv's expiry is at or before now, matching
// i18n/remote/remote.go's `!e.expiresAt.After(now)` check. A zero expiry
// means persist (no expiry, see PutTTL) and is never expired. The boundary
// is therefore: live while now is strictly before expiresAt, expired at the
// exact deadline and after. Callers must copy []byte-style values themselves;
// the cache stores V as-is with no cloning.
func expired[V any](tv ttlValue[V], now time.Time) bool {
	if tv.expiresAt.IsZero() {
		return false
	}
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

	if expired(tv, t.nowTime()) {
		t.c.Delete(k)
		var zero V
		return zero, false
	}

	return tv.val, true
}

// Put inserts or updates the value for k, resetting its expiry to now+ttl
// (the cache's default, constructor-time TTL), and marks it
// most-recently-used. It is a convenience wrapper around PutTTL using the
// cache's default ttl; see PutTTL to store an entry with a different TTL.
func (t *TTLCache[K, V]) Put(k K, v V) {
	t.PutTTL(k, v, t.ttl)
}

// PutTTL inserts or updates the value for k, resetting its expiry to
// now+ttl using the ttl given here rather than the cache's default, and
// marks it most-recently-used. If the cache is over capacity afterward, the
// least-recently-used entry is evicted (see Cache.Put); the OnEvict
// callback, if registered, fires with the evicted entry's unwrapped value.
//
// A non-positive ttl means persist: the entry is stored with no expiry and
// never expires until explicitly deleted, evicted, or overwritten. This
// matches core/cache memory-adapter semantics where ttl<=0 persists. No
// current in-repo caller passes a non-positive TTL (i18n/remote always uses
// positive posTTL/negTTL), so this sentinel changes no existing behavior.
//
// Values are stored as-is with no copy; callers holding mutable V (e.g.
// []byte) must copy on store and after retrieval.
func (t *TTLCache[K, V]) PutTTL(k K, v V, ttl time.Duration) {
	var exp time.Time
	if ttl > 0 {
		exp = t.nowTime().Add(ttl)
	}
	t.c.Put(k, ttlValue[V]{val: v, expiresAt: exp})
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

	if expired(tv, t.nowTime()) {
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
