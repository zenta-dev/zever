package sqlite

import (
	"database/sql"
	"errors"

	"github.com/zenta-dev/zever/shared/lrucache"
)

// defaultStmtCacheSize bounds the stmt cache when newStmtCache gets a
// non-positive capacity. It intentionally differs from (and overrides)
// lrucache.New's own generic default, matching this package's previous
// hand-rolled stmtCache default.
const defaultStmtCacheSize = 256

// newStmtCache returns a bounded, concurrency-safe LRU cache of prepared
// statements keyed by SQL text, backed by internal/lrucache. A non-positive
// capacity means defaultStmtCacheSize.
//
// A *sql.Stmt from DB.PrepareContext is not bound to one connection:
// database/sql re-prepares it on whichever pooled connection serves a call,
// so one adapter-level cache is correct with no connection-affinity logic.
// Statements are safe for concurrent use by multiple goroutines; the cache's
// own internal lock guards only the LRU bookkeeping.
//
// Eviction policy: true LRU -- Get refreshes recency, Put/GetOrCompute evict
// the least-recently-used entry past capacity, and the registered OnEvict
// callback closes it so statement handles don't leak as memory stays
// bounded.
func newStmtCache(capacity int) *lrucache.Cache[string, *sql.Stmt] {
	if capacity <= 0 {
		capacity = defaultStmtCacheSize
	}

	return lrucache.New[string, *sql.Stmt](capacity, lrucache.WithOnEvict(func(_ string, stmt *sql.Stmt) {
		_ = stmt.Close()
	}))
}

// closeAllStmts closes every statement currently cached in c and empties it.
// Errors join via errors.Join so one bad Close never skips the rest.
//
// This uses Cache.Clear's own callback parameter, not the OnEvict callback
// registered by newStmtCache, to perform the close: per lrucache's
// documented contract, Clear does NOT also invoke OnEvict for the entries it
// removes, so each cached statement is closed exactly once here, not twice.
func closeAllStmts(c *lrucache.Cache[string, *sql.Stmt]) error {
	var errs []error

	c.Clear(func(_ string, stmt *sql.Stmt) {
		// errors.Join discards nils, so append unconditionally: one bad
		// Close never prevents the rest from being closed.
		errs = append(errs, stmt.Close())
	})

	return errors.Join(errs...)
}
