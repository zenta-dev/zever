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

	return stmt
}

func TestNewStmtCache_InvalidCapacityDefaults(t *testing.T) {
	t.Parallel()

	for _, capacity := range []int{0, -1, -256} {
		c := newStmtCache(capacity)
		if c.capacity != defaultStmtCacheSize {
			t.Errorf("newStmtCache(%d).capacity = %d, want %d", capacity, c.capacity, defaultStmtCacheSize)
		}

		if got := c.Len(); got != 0 {
			t.Errorf("newStmtCache(%d).Len() = %d, want 0", capacity, got)
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
	kept := c.Put("SELECT 1", stmt)
	if kept != stmt {
		t.Fatal("Put should keep caller's stmt on first insert")
	}

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

func TestStmtCache_PutDuplicateRaceClosesLoser(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	c := newStmtCache(4)

	first := prepare(t, db, "SELECT 1")
	second := prepare(t, db, "SELECT 1")

	keptFirst := c.Put("SELECT 1", first)
	if keptFirst != first {
		t.Fatal("first Put should win")
	}

	keptSecond := c.Put("SELECT 1", second)
	if keptSecond != first {
		t.Fatal("duplicate Put should keep existing stmt")
	}

	// Loser must be closed: any use errors.
	if _, err := second.ExecContext(context.Background()); err == nil {
		t.Fatal("loser stmt use after duplicate Put should error (closed)")
	}

	got, ok := c.Get("SELECT 1")
	if !ok || got != first {
		t.Fatal("cache should still hold first stmt after duplicate Put")
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

	// Evicted stmt must be closed: use errors.
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

	if err := c.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}

	if got := c.Len(); got != 0 {
		t.Fatalf("Len() after CloseAll = %d, want 0", got)
	}

	if _, ok := c.Get("q1"); ok {
		t.Fatal("Get after CloseAll = hit, want miss")
	}

	// Cached stmts closed: use errors.
	if _, err := s1.ExecContext(context.Background()); err == nil {
		t.Fatal("s1 use after CloseAll should error (closed)")
	}

	if _, err := s2.ExecContext(context.Background()); err == nil {
		t.Fatal("s2 use after CloseAll should error (closed)")
	}

	// Second CloseAll on empty cache is nil-safe.
	if err := c.CloseAll(); err != nil {
		t.Fatalf("second CloseAll: %v", err)
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

func TestStmtCache_corruptEntryDefensive(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	c := newStmtCache(4)

	// Poison the internals with a non-*entry element: the comma-ok
	// assertions must degrade gracefully instead of panicking.
	c.ll.PushFront("not-an-entry")
	c.items["bogus"] = c.ll.Front()

	if _, ok := c.Get("bogus"); ok {
		t.Fatal("Get on corrupt entry = hit, want miss")
	}

	stmt := prepare(t, db, "SELECT 1")
	if got := c.Put("bogus", stmt); got != stmt {
		t.Fatal("Put on corrupt entry must return caller's stmt")
	}
	_ = stmt.Close()
}
