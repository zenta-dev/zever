package lrucache

import (
	"container/list"
	"sync"
)

// defaultCapacity is used by New when capacity <= 0. A non-positive capacity
// is treated as "use a sensible default" rather than a caller error, so a
// zero-value config field (e.g. an unset MaxEntries) degrades gracefully
// instead of panicking or producing a cache that can never hold anything.
const defaultCapacity = 128

// entry is the value stored in each list.Element.
type entry[K comparable, V any] struct {
	key K
	val V
}

// Option configures a Cache constructed by New.
type Option[K comparable, V any] func(*Cache[K, V])

// WithOnEvict registers fn to be called whenever an entry is evicted from
// the cache due to exceeding its capacity (never on an explicit Delete; see
// Cache.Delete). fn is invoked synchronously, after the cache's internal
// lock has been released, so it is safe for fn to call back into the same
// Cache (e.g. Get/Put/Delete) without deadlocking. Because it runs
// synchronously and outside the lock, a slow or blocking fn will delay the
// Put call that triggered the eviction but will not hold up other
// goroutines contending for the cache.
func WithOnEvict[K comparable, V any](fn func(K, V)) Option[K, V] {
	return func(c *Cache[K, V]) {
		c.onEvict = fn
	}
}

// Cache is a fixed-capacity, generic, thread-safe LRU cache backed by
// container/list (for recency order) and a map (for O(1) lookup). It uses a
// plain sync.Mutex rather than sync.RWMutex: Get mutates recency order (it
// moves the accessed entry to the front of the list), so even read-only
// lookups require exclusive access.
//
// The zero value is not usable; construct with New.
type Cache[K comparable, V any] struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[K]*list.Element
	onEvict  func(K, V)
}

// New creates a Cache with the given capacity. capacity <= 0 is replaced
// with a default capacity (currently 128) rather than panicking, so callers
// that forward a zero-value config field get a working, bounded cache
// instead of a crash or an unbounded one.
func New[K comparable, V any](capacity int, opts ...Option[K, V]) *Cache[K, V] {
	if capacity <= 0 {
		capacity = defaultCapacity
	}

	c := &Cache[K, V]{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[K]*list.Element, capacity),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Get returns the value stored for k and moves it to the front of the
// recency list (marking it most-recently-used). The second return value
// reports whether k was present.
func (c *Cache[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[k]
	if !ok {
		var zero V
		return zero, false
	}

	c.ll.MoveToFront(el)
	return el.Value.(*entry[K, V]).val, true
}

// Put inserts or updates the value for k and marks it most-recently-used.
// If the cache is over capacity afterward, the least-recently-used entry is
// evicted and, if an OnEvict callback was registered via WithOnEvict, it is
// invoked with the evicted key/value after the internal lock is released.
func (c *Cache[K, V]) Put(k K, v V) {
	var (
		evictKey K
		evictVal V
		evicted  bool
	)

	c.mu.Lock()
	if el, ok := c.items[k]; ok {
		el.Value.(*entry[K, V]).val = v
		c.ll.MoveToFront(el)
	} else {
		el := c.ll.PushFront(&entry[K, V]{key: k, val: v})
		c.items[k] = el

		if c.ll.Len() > c.capacity {
			back := c.ll.Back()
			if back != nil {
				be := back.Value.(*entry[K, V])
				c.ll.Remove(back)
				delete(c.items, be.key)
				evictKey, evictVal, evicted = be.key, be.val, true
			}
		}
	}
	c.mu.Unlock()

	if evicted && c.onEvict != nil {
		c.onEvict(evictKey, evictVal)
	}
}

// Delete removes k from the cache and returns its value, if present.
//
// Delete does NOT invoke the OnEvict callback. OnEvict exists to let a
// capacity-bound eviction release a resource the cache no longer has room
// for (e.g. closing a pooled *sql.Stmt); an explicit Delete is the caller
// deliberately discarding the entry, and the caller already has the
// returned value in hand to dispose of however it sees fit. Firing OnEvict
// here as well would risk a double-close/double-free for callers that both
// use the returned value and rely on OnEvict.
func (c *Cache[K, V]) Delete(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[k]
	if !ok {
		var zero V
		return zero, false
	}

	e := el.Value.(*entry[K, V])
	c.ll.Remove(el)
	delete(c.items, k)
	return e.val, true
}

// Len returns the number of entries currently in the cache.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ll.Len()
}
