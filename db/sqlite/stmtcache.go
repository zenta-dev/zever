package sqlite

import (
	"container/list"
	"database/sql"
	"errors"
	"sync"
)

// defaultStmtCacheSize bounds the stmtCache when newStmtCache gets a
// non-positive capacity.
const defaultStmtCacheSize = 256

// entry pairs a query text with its prepared statement.
type entry struct {
	query string
	stmt  *sql.Stmt
}

// stmtCache is a bounded, concurrency-safe LRU cache of prepared statements,
// keyed by SQL text.
//
// A *sql.Stmt from DB.PrepareContext is not bound to one connection:
// database/sql re-prepares it on whichever pooled connection serves a call,
// so one adapter-level cache is correct with no connection-affinity logic.
// Statements are safe for concurrent use by multiple goroutines; the mutex
// below guards only the LRU bookkeeping.
type stmtCache struct {
	mu       sync.Mutex
	capacity int
	ll       *list.List
	items    map[string]*list.Element
}

// newStmtCache returns a stmtCache holding at most capacity statements.
// A non-positive capacity means defaultStmtCacheSize. Eviction policy: true
// LRU — Get refreshes recency, Put evicts the least-recently-used entry past
// capacity and closes it, so memory stays bounded.
func newStmtCache(capacity int) *stmtCache {
	if capacity <= 0 {
		capacity = defaultStmtCacheSize
	}

	return &stmtCache{
		capacity: capacity,
		ll:       list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

// Get returns the cached statement for query, moving it to the front of the
// LRU order, or (nil, false) on a miss.
func (c *stmtCache) Get(query string) (*sql.Stmt, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[query]
	if !ok {
		return nil, false
	}

	c.ll.MoveToFront(el)

	e, ok := el.Value.(*entry)
	if !ok {
		return nil, false
	}

	return e.stmt, true
}

// Put inserts stmt for query, evicting and closing the least-recently-used
// entry past capacity. On a duplicate query (two concurrent misses both
// prepared the same text) the new stmt is closed and the cached one kept.
// Returns the kept stmt; callers must use it, not the passed-in pointer.
func (c *stmtCache) Put(query string, stmt *sql.Stmt) *sql.Stmt {
	c.mu.Lock()

	if el, ok := c.items[query]; ok {
		c.ll.MoveToFront(el)
		c.mu.Unlock()

		_ = stmt.Close()

		if kept, ok := el.Value.(*entry); ok {
			return kept.stmt
		}

		return stmt
	}

	el := c.ll.PushFront(&entry{query: query, stmt: stmt})
	c.items[query] = el

	var evicted *entry

	if c.ll.Len() > c.capacity {
		if oldest := c.ll.Back(); oldest != nil {
			c.ll.Remove(oldest)

			if e, ok := oldest.Value.(*entry); ok {
				evicted = e
				delete(c.items, e.query)
			}
		}
	}

	c.mu.Unlock()

	if evicted != nil {
		_ = evicted.stmt.Close()
	}

	return stmt
}

// CloseAll closes every cached statement and empties the cache. Errors join
// via errors.Join so one bad Close never skips the rest.
func (c *stmtCache) CloseAll() error {
	c.mu.Lock()
	entries := make([]*entry, 0, len(c.items))

	for el := c.ll.Front(); el != nil; el = el.Next() {
		if e, ok := el.Value.(*entry); ok {
			entries = append(entries, e)
		}
	}

	c.ll.Init()
	c.items = make(map[string]*list.Element)
	c.mu.Unlock()

	errs := make([]error, 0, len(entries))

	for _, e := range entries {
		// errors.Join discards nils, so append unconditionally: one bad
		// Close never prevents the rest from being closed.
		errs = append(errs, e.stmt.Close())
	}

	return errors.Join(errs...)
}

// Len reports how many statements are cached.
func (c *stmtCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ll.Len()
}
