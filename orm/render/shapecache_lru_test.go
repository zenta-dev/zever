package render

import (
	"strconv"
	"sync"
	"testing"

	sqlited "github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestShapeCacheEvictsLRU proves the cache evicts only the
// least-recently-used entry on overflow instead of dropping everything:
// after filling the cache, re-hitting shape 0 makes it most-recently-used,
// so inserting one more shape must evict shape 1 while shape 0 still hits.
func TestShapeCacheEvictsLRU(t *testing.T) {
	d := sqlited.New()

	resetShapeCache()

	cols := func(i int) []string { return []string{"id" + strconv.Itoa(i)} }

	for i := 0; i < shapeCacheCapacity; i++ {
		if _, _, err := Select(d, "users", cols(i), Node{}, nil, 0, 0); err != nil {
			t.Fatalf("fill shape %d: %v", i, err)
		}
	}

	// Re-hit shape 0 so it is most-recently-used, not least.
	if _, _, err := Select(d, "users", cols(0), Node{}, nil, 0, 0); err != nil {
		t.Fatalf("refresh shape 0: %v", err)
	}

	hitsBefore := shapeCacheHits.Load()

	// One more distinct shape forces exactly one eviction.
	if _, _, err := Select(d, "users", cols(shapeCacheCapacity), Node{}, nil, 0, 0); err != nil {
		t.Fatalf("overflow shape: %v", err)
	}

	if n := shapeCacheLen(); n > shapeCacheCapacity {
		t.Fatalf("cache holds %d entries, capacity %d", n, shapeCacheCapacity)
	}

	if n := shapeCacheLen(); n < shapeCacheCapacity-shapeCacheShards {
		t.Fatalf("cache holds %d entries, want near capacity %d (single-entry eviction per stripe at most)", n, shapeCacheCapacity)
	}

	// Shape 0 was refreshed above, so it must survive the eviction.
	if _, _, err := Select(d, "users", cols(0), Node{}, nil, 0, 0); err != nil {
		t.Fatalf("shape 0 after eviction: %v", err)
	}

	if got := shapeCacheHits.Load(); got != hitsBefore+1 {
		t.Fatalf("shape 0 re-render produced no cache hit (hits %d, want %d)", got, hitsBefore+1)
	}
}

// TestShapeCacheConcurrentAccess hammers the cache from many goroutines
// with a mix of shared hot shapes and distinct churn shapes; the race
// detector guards the access, and the length bound must hold after.
func TestShapeCacheConcurrentAccess(t *testing.T) {
	d := sqlited.New()

	resetShapeCache()

	var wg sync.WaitGroup

	for w := 0; w < 8; w++ {
		wg.Add(1)

		go func(w int) {
			defer wg.Done()

			for i := 0; i < 50; i++ {
				// Hot shape shared by all workers.
				if _, _, err := Select(d, "users", []string{"id", "name"}, Node{}, nil, 0, 0); err != nil {
					t.Error(err)

					return
				}

				// Distinct churn shape per worker iteration.
				cols := []string{"id", "name", "w" + strconv.Itoa(w) + "c" + strconv.Itoa(i)}
				if _, _, err := Select(d, "users", cols, Node{}, nil, 0, 0); err != nil {
					t.Error(err)

					return
				}
			}
		}(w)
	}

	wg.Wait()

	if n := shapeCacheLen(); n > shapeCacheCapacity {
		t.Fatalf("cache holds %d entries, capacity %d", n, shapeCacheCapacity)
	}

	if shapeCacheHits.Load() == 0 {
		t.Fatal("concurrent hot-shape hits produced no cache hits")
	}
}
