package lrucache

import "time"

// This file extends TTLCache with the atomic primitives a core/cache adapter
// needs to implement the full cache contract without a parallel side
// structure. All methods are goroutine-safe via the internal cache mutex;
// each check-and-mutate holds that single lock, so concurrent callers never
// observe a torn read-modify-write. Close/ErrClosed stay in the adapter: this
// library owns no goroutine and no closed state.
//
// Value copies: the cache stores V as-is and returns the stored V as-is,
// with no cloning. Callers with mutable V (e.g. []byte) must copy on store
// and after retrieval.
//
// Persist sentinel: any ttl <= 0 stores the entry with a zero expiry that
// never expires (see PutTTL). Expiry boundary is !expiresAt.After(now): an
// entry is live while now is strictly before its deadline and expired at the
// exact deadline and after. Zero expiry never expires.

// GetWithExpiry returns the value stored for k plus its absolute expiry, and
// moves k to the front of the recency list. expiresAt is zero for persist
// entries (stored with ttl <= 0). It reports ok=false when k is absent or
// past its TTL; an expired entry found this way is removed as a side effect
// (lazy expiry), exactly like Get.
func (t *TTLCache[K, V]) GetWithExpiry(k K) (V, time.Time, bool) {
	now := t.nowTime()

	t.c.mu.Lock()
	defer t.c.mu.Unlock()

	el, ok := t.c.items[k]
	if !ok {
		var zero V
		return zero, time.Time{}, false
	}

	tv := el.Value.(*entry[K, ttlValue[V]]).val //nolint:forcetypeassert // list elements are only ever pushed as *entry[K, ttlValue[V]]
	if expired(tv, now) {
		t.c.ll.Remove(el)
		delete(t.c.items, k)
		var zero V
		return zero, time.Time{}, false
	}

	t.c.ll.MoveToFront(el)
	return tv.val, tv.expiresAt, true
}

// SetIfAbsent inserts v for k with the given ttl only when no live entry
// exists, and reports whether it stored. An absent key or a present-but-
// expired key is replaced (expired entries are purged inline); a live entry
// is left untouched and SetIfAbsent returns false without refreshing its
// recency or expiry. A non-positive ttl stores a persist entry with no
// expiry. A successful insert marks k most-recently-used and is subject to
// the same capacity eviction (and OnEvict callback) as Put.
func (t *TTLCache[K, V]) SetIfAbsent(k K, v V, ttl time.Duration) bool {
	now := t.nowTime()

	var exp time.Time
	if ttl > 0 {
		exp = now.Add(ttl)
	}

	var (
		evictKey K
		evictVal ttlValue[V]
		evicted  bool
	)

	t.c.mu.Lock()
	if el, ok := t.c.items[k]; ok {
		tv := el.Value.(*entry[K, ttlValue[V]]).val //nolint:forcetypeassert // list elements are only ever pushed as *entry[K, ttlValue[V]]
		if !expired(tv, now) {
			t.c.mu.Unlock()
			return false
		}
		t.c.ll.Remove(el)
		delete(t.c.items, k)
	}

	el := t.c.ll.PushFront(&entry[K, ttlValue[V]]{key: k, val: ttlValue[V]{val: v, expiresAt: exp}})
	t.c.items[k] = el

	if t.c.ll.Len() > t.c.capacity {
		if back := t.c.ll.Back(); back != nil {
			be := back.Value.(*entry[K, ttlValue[V]]) //nolint:forcetypeassert // list elements are only ever pushed as *entry[K, ttlValue[V]]
			t.c.ll.Remove(back)
			delete(t.c.items, be.key)
			evictKey, evictVal, evicted = be.key, be.val, true
		}
	}
	t.c.mu.Unlock()

	if evicted && t.c.onEvict != nil {
		t.c.onEvict(evictKey, evictVal)
	}
	return true
}

// CompareAndDelete removes k only when its live value satisfies
// equal(stored, expected). It reports false when k is missing, expired
// (purged inline), mismatched, or equal is nil. Like Delete, it never
// invokes the OnEvict callback.
func (t *TTLCache[K, V]) CompareAndDelete(k K, expected V, equal func(a, b V) bool) bool {
	now := t.nowTime()

	t.c.mu.Lock()
	defer t.c.mu.Unlock()

	el, ok := t.c.items[k]
	if !ok {
		return false
	}

	tv := el.Value.(*entry[K, ttlValue[V]]).val //nolint:forcetypeassert // list elements are only ever pushed as *entry[K, ttlValue[V]]
	if expired(tv, now) {
		t.c.ll.Remove(el)
		delete(t.c.items, k)
		return false
	}

	if equal == nil || !equal(tv.val, expected) {
		return false
	}

	t.c.ll.Remove(el)
	delete(t.c.items, k)
	return true
}

// CompareAndSwap replaces k's value with replacement only when its live
// value satisfies equal(stored, expected), preserving the existing expiry
// untouched, and marks k most-recently-used. It reports false when k is
// missing, expired (purged inline), mismatched, or equal is nil. Combined
// with GetWithExpiry in a retry loop, this gives atomic read-modify-write
// (e.g. integer increment preserving TTL) without the library knowing V's
// encoding.
func (t *TTLCache[K, V]) CompareAndSwap(k K, expected, replacement V, equal func(a, b V) bool) bool {
	now := t.nowTime()

	t.c.mu.Lock()
	defer t.c.mu.Unlock()

	el, ok := t.c.items[k]
	if !ok {
		return false
	}

	e := el.Value.(*entry[K, ttlValue[V]]) //nolint:forcetypeassert // list elements are only ever pushed as *entry[K, ttlValue[V]]
	if expired(e.val, now) {
		t.c.ll.Remove(el)
		delete(t.c.items, k)
		return false
	}

	if equal == nil || !equal(e.val.val, expected) {
		return false
	}

	e.val.val = replacement
	t.c.ll.MoveToFront(el)
	return true
}

// CompareAndExtend renews k's TTL to now+ttl only when its live value
// satisfies equal(stored, expected), and marks k most-recently-used. A
// non-positive ttl clears the expiry (persist). It reports false when k is
// missing, expired (purged inline), mismatched, or equal is nil.
func (t *TTLCache[K, V]) CompareAndExtend(k K, expected V, ttl time.Duration, equal func(a, b V) bool) bool {
	now := t.nowTime()

	t.c.mu.Lock()
	defer t.c.mu.Unlock()

	el, ok := t.c.items[k]
	if !ok {
		return false
	}

	e := el.Value.(*entry[K, ttlValue[V]]) //nolint:forcetypeassert // list elements are only ever pushed as *entry[K, ttlValue[V]]
	if expired(e.val, now) {
		t.c.ll.Remove(el)
		delete(t.c.items, k)
		return false
	}

	if equal == nil || !equal(e.val.val, expected) {
		return false
	}

	var exp time.Time
	if ttl > 0 {
		exp = now.Add(ttl)
	}
	e.val.expiresAt = exp
	t.c.ll.MoveToFront(el)
	return true
}

// PurgeExpired removes every entry past its TTL in a single locked pass and
// returns the count removed. Persist entries (zero expiry) are always kept.
// It never invokes the OnEvict callback (expiry purge, not capacity
// eviction). Adapter janitors call this on a ticker instead of ranging keys
// themselves.
func (t *TTLCache[K, V]) PurgeExpired() int {
	now := t.nowTime()

	t.c.mu.Lock()
	defer t.c.mu.Unlock()

	purged := 0
	for k, el := range t.c.items {
		tv := el.Value.(*entry[K, ttlValue[V]]).val //nolint:forcetypeassert // list elements are only ever pushed as *entry[K, ttlValue[V]]
		if !expired(tv, now) {
			continue
		}
		t.c.ll.Remove(el)
		delete(t.c.items, k)
		purged++
	}
	return purged
}

// Range calls fn once per entry in the cache with its absolute expiry (zero
// for persist entries), stopping early when fn returns false. The visited
// set is a snapshot taken under the internal lock: order is unspecified, it
// may include entries that have expired but not yet been purged, and fn runs
// after the lock is released, so fn may safely call back into this cache.
// Range never purges, never changes recency, and never invokes OnEvict.
func (t *TTLCache[K, V]) Range(fn func(k K, expiresAt time.Time) bool) {
	if fn == nil {
		return
	}

	type pair struct {
		k   K
		exp time.Time
	}

	t.c.mu.Lock()
	snap := make([]pair, 0, len(t.c.items))
	for k, el := range t.c.items {
		tv := el.Value.(*entry[K, ttlValue[V]]).val //nolint:forcetypeassert // list elements are only ever pushed as *entry[K, ttlValue[V]]
		snap = append(snap, pair{k: k, exp: tv.expiresAt})
	}
	t.c.mu.Unlock()

	for _, p := range snap {
		if !fn(p.k, p.exp) {
			return
		}
	}
}
