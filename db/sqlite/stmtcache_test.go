// Resource ownership in this file lives in the shared prepare()/openTestDB
// helpers (t.Cleanup) and in test bodies that explicitly close a losing
// stmt to assert on it, not at each stmtCache.Get/GetOrCompute/prepare call
// site, so sqlclosecheck cannot see the close and flags every call here.
//
//nolint:sqlclosecheck // see file doc above
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	db.SetMaxOpenConns(1)

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func prepare(t *testing.T, db *sql.DB, query string) *sql.Stmt {
	t.Helper()

	stmt, err := db.PrepareContext(context.Background(), query)
	if err != nil {
		t.Fatalf("Prepare(%q): %v", query, err)
	}

	t.Cleanup(func() { _ = stmt.Close() })

	return stmt
}

func TestNewStmtCache_InvalidCapacityDefaults(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)

	for _, capacity := range []int{0, -1, -256} {
		c := newStmtCache(capacity)

		if got := c.Len(); got != 0 {
			t.Errorf("newStmtCache(%d).Len() = %d, want 0", capacity, got)
		}

		// Fill past what a small explicit capacity would allow; only the
		// defaultStmtCacheSize ceiling should apply, observable via Len
		// never exceeding it (the cache's capacity field itself is no
		// longer directly reachable now that it's a plain lrucache.Cache).
		for i := 0; i < defaultStmtCacheSize+10; i++ {
			c.Put(fmt.Sprintf("SELECT %d", i), prepare(t, db, "SELECT 1"))
		}

		if got := c.Len(); got != defaultStmtCacheSize {
			t.Errorf("newStmtCache(%d) after overfill: Len() = %d, want %d", capacity, got, defaultStmtCacheSize)
		}

		if err := closeAllStmts(c); err != nil {
			t.Errorf("closeAllStmts: %v", err)
		}
	}
}

func TestStmtCache_GetMiss(t *testing.T) {
	t.Parallel()

	c := newStmtCache(4)

	if _, ok := c.Get("SELECT 1"); ok {
		t.Fatal("Get on empty cache = hit, want miss")
	}
}

func TestStmtCache_HitAfterPut(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	c := newStmtCache(4)

	stmt := prepare(t, db, "SELECT 1")
	c.Put("SELECT 1", stmt)

	got, ok := c.Get("SELECT 1")
	if !ok {
		t.Fatal("Get after Put = miss, want hit")
	}

	if got != stmt {
		t.Fatal("Get returned different stmt than cached")
	}

	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
}

// TestStmtCache_GetOrComputeDuplicateRaceClosesLoser exercises the same
// "duplicate insert race" contract the old hand-rolled stmtCache.Put used to
// own directly: when two goroutines race to prepare+cache the same query,
// only one stmt ends up cached, and the caller of the losing GetOrCompute
// call is told (via loaded == true) to close its own redundant stmt. This is
// exactly the pattern sqlite.go's adapter.Prepare uses.
func TestStmtCache_GetOrComputeDuplicateRaceClosesLoser(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	c := newStmtCache(4)

	first := prepare(t, db, "SELECT 1")
	second := prepare(t, db, "SELECT 1")

	keptFirst, loadedFirst := c.GetOrCompute("SELECT 1", func() *sql.Stmt { return first })
	if loadedFirst || keptFirst != first {
		t.Fatal("first GetOrCompute should win and store its own stmt")
	}

	keptSecond, loadedSecond := c.GetOrCompute("SELECT 1", func() *sql.Stmt { return second })
	if !loadedSecond || keptSecond != first {
		t.Fatal("duplicate GetOrCompute should report loaded=true and keep existing stmt")
	}

	// Per adapter.Prepare's pattern, the loser must close its own stmt.
	_ = second.Close()

	// Loser must be closed: any use errors.
	if _, err := second.ExecContext(context.Background()); err == nil {
		t.Fatal("loser stmt use after duplicate GetOrCompute should error (closed)")
	}

	got, ok := c.Get("SELECT 1")
	if !ok || got != first {
		t.Fatal("cache should still hold first stmt after duplicate GetOrCompute")
	}

	if got := c.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1", got)
	}
}

func TestStmtCache_EvictionClosesEvicted(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	c := newStmtCache(2)

	s1 := prepare(t, db, "SELECT 1")
	s2 := prepare(t, db, "SELECT 2")
	s3 := prepare(t, db, "SELECT 3")

	c.Put("q1", s1)
	c.Put("q2", s2)
	// Touch q1 so q2 is LRU... actually make q1 MRU, q2 LRU.
	if _, ok := c.Get("q1"); !ok {
		t.Fatal("expected q1 hit")
	}

	c.Put("q3", s3)

	if _, ok := c.Get("q2"); ok {
		t.Fatal("q2 should have been evicted")
	}

	// Evicted stmt must be closed via the OnEvict callback registered by
	// newStmtCache: use errors.
	if _, err := s2.ExecContext(context.Background()); err == nil {
		t.Fatal("evicted stmt use should error (closed)")
	}

	if got := c.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
}

func TestStmtCache_CloseAll(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	c := newStmtCache(4)

	s1 := prepare(t, db, "SELECT 1")
	s2 := prepare(t, db, "SELECT 2")

	c.Put("q1", s1)
	c.Put("q2", s2)

	if err := closeAllStmts(c); err != nil {
		t.Fatalf("closeAllStmts: %v", err)
	}

	if got := c.Len(); got != 0 {
		t.Fatalf("Len() after closeAllStmts = %d, want 0", got)
	}

	if _, ok := c.Get("q1"); ok {
		t.Fatal("Get after closeAllStmts = hit, want miss")
	}

	// Cached stmts closed: use errors.
	if _, err := s1.ExecContext(context.Background()); err == nil {
		t.Fatal("s1 use after closeAllStmts should error (closed)")
	}

	if _, err := s2.ExecContext(context.Background()); err == nil {
		t.Fatal("s2 use after closeAllStmts should error (closed)")
	}

	// Second closeAllStmts on empty cache is nil-safe.
	if err := closeAllStmts(c); err != nil {
		t.Fatalf("second closeAllStmts: %v", err)
	}
}

// TestStmtCache_CloseAllNoDoubleClose confirms that closeAllStmts (via
// Cache.Clear's own callback) does not also trigger the OnEvict callback
// registered by newStmtCache -- otherwise every cached stmt would be closed
// twice, and a *sql.Stmt double-Close would surface as a spurious error from
// closeAllStmts's errors.Join.
func TestStmtCache_CloseAllNoDoubleClose(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	c := newStmtCache(4)

	c.Put("q1", prepare(t, db, "SELECT 1"))
	c.Put("q2", prepare(t, db, "SELECT 2"))

	if err := closeAllStmts(c); err != nil {
		t.Fatalf("closeAllStmts: %v, want nil (a double-close would report an error here)", err)
	}
}

func TestStmtCache_ConcurrentGetPut(t *testing.T) {
	db := openTestDB(t)
	c := newStmtCache(16)

	var wg sync.WaitGroup

	for g := 0; g < 8; g++ {
		wg.Add(1)

		go func(g int) {
			defer wg.Done()

			q := fmt.Sprintf("SELECT %d", g%4)

			for i := 0; i < 25; i++ {
				if _, ok := c.Get(q); !ok {
					stmt, err := db.PrepareContext(context.Background(), "SELECT 1")
					if err != nil {
						t.Errorf("Prepare: %v", err)
						return
					}

					c.Put(q, stmt)
				}
			}
		}(g)
	}

	wg.Wait()

	if got := c.Len(); got > 16 {
		t.Fatalf("Len() = %d, want <= 16", got)
	}
}
