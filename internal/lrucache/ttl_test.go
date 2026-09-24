package lrucache

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually-advanced clock for deterministic TTL tests.
type fakeClock struct {
	now time.Time
}

// Now returns the clock's current time.
func (f *fakeClock) Now() time.Time { return f.now }

// Advance moves the clock forward by d.
func (f *fakeClock) Advance(d time.Duration) { f.now = f.now.Add(d) }

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1_700_000_000, 0)}
}

func TestTTLCache_GetPut(t *testing.T) {
	tc := NewTTL[string, int](4, time.Hour)

	if v, ok := tc.Get("missing"); ok || v != 0 {
		t.Fatalf("Get(missing) = (%v, %v), want (0, false)", v, ok)
	}

	tc.Put("a", 1)
	if v, ok := tc.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = (%v, %v), want (1, true)", v, ok)
	}
}

func TestTTLCache_ExpiryOnGet(t *testing.T) {
	fc := newFakeClock()
	tc := NewTTL[string, int](4, time.Millisecond)
	tc.now = fc.Now
	tc.Put("a", 1)

	fc.Advance(5 * time.Millisecond)

	v, ok := tc.Get("a")
	if ok || v != 0 {
		t.Fatalf("Get(a) after expiry = (%v, %v), want (0, false)", v, ok)
	}
	// Lazy expiry should have purged the stale entry.
	if tc.Len() != 0 {
		t.Fatalf("Len() = %d, want 0 after lazy purge", tc.Len())
	}
}

func TestTTLCache_PutRefreshesExpiry(t *testing.T) {
	fc := newFakeClock()
	tc := NewTTL[string, int](4, 20*time.Millisecond)
	tc.now = fc.Now
	tc.Put("a", 1)

	fc.Advance(10 * time.Millisecond)
	tc.Put("a", 2) // refresh expiry

	fc.Advance(15 * time.Millisecond) // 25ms since first Put, 15ms since refresh

	v, ok := tc.Get("a")
	if !ok || v != 2 {
		t.Fatalf("Get(a) = (%v, %v), want (2, true); Put should reset TTL", v, ok)
	}
}

func TestTTLCache_Delete(t *testing.T) {
	t.Run("delete present unexpired key", func(t *testing.T) {
		tc := NewTTL[string, int](4, time.Hour)
		tc.Put("a", 1)
		v, ok := tc.Delete("a")
		if !ok || v != 1 {
			t.Fatalf("Delete(a) = (%v, %v), want (1, true)", v, ok)
		}
		if _, ok := tc.Get("a"); ok {
			t.Fatalf("expected a to be gone after Delete")
		}
	})

	t.Run("delete missing key", func(t *testing.T) {
		tc := NewTTL[string, int](4, time.Hour)
		v, ok := tc.Delete("missing")
		if ok || v != 0 {
			t.Fatalf("Delete(missing) = (%v, %v), want (0, false)", v, ok)
		}
	})

	t.Run("delete expired key reports absent", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, time.Millisecond)
		tc.now = fc.Now
		tc.Put("a", 1)
		fc.Advance(5 * time.Millisecond)
		v, ok := tc.Delete("a")
		if ok || v != 0 {
			t.Fatalf("Delete(expired a) = (%v, %v), want (0, false)", v, ok)
		}
	})

	t.Run("delete does not invoke OnEvict", func(t *testing.T) {
		fired := false
		tc := NewTTL[string, int](4, time.Hour, WithOnEvict[string, int](func(_ string, _ int) {
			fired = true
		}))
		tc.Put("a", 1)
		tc.Delete("a")
		if fired {
			t.Fatalf("explicit Delete must not invoke OnEvict")
		}
	})
}

func TestTTLCache_CapacityEviction(t *testing.T) {
	var mu sync.Mutex
	var evicted []string

	tc := NewTTL[string, int](2, time.Hour, WithOnEvict[string, int](func(k string, _ int) {
		mu.Lock()
		evicted = append(evicted, k)
		mu.Unlock()
	}))

	tc.Put("a", 1)
	tc.Put("b", 2)
	tc.Get("a")    // a is now most-recently-used
	tc.Put("c", 3) // should evict b, the LRU entry

	if _, ok := tc.Get("b"); ok {
		t.Fatalf("expected b to be evicted by capacity")
	}
	if v, ok := tc.Get("a"); !ok || v != 1 {
		t.Fatalf("expected a to survive, got (%v, %v)", v, ok)
	}
	if v, ok := tc.Get("c"); !ok || v != 3 {
		t.Fatalf("expected c present, got (%v, %v)", v, ok)
	}
	if tc.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", tc.Len())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(evicted) != 1 || evicted[0] != "b" {
		t.Fatalf("evicted = %v, want [b]", evicted)
	}
}

func TestNewTTL_NonPositiveCapacityUsesDefault(t *testing.T) {
	tc := NewTTL[int, int](0, time.Hour)
	for i := 0; i < defaultCapacity+10; i++ {
		tc.Put(i, i)
	}
	if tc.Len() != defaultCapacity {
		t.Fatalf("Len() = %d, want %d", tc.Len(), defaultCapacity)
	}
}

func TestTTLCache_Len(t *testing.T) {
	tc := NewTTL[string, int](4, time.Hour)
	if tc.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", tc.Len())
	}
	tc.Put("a", 1)
	tc.Put("b", 2)
	if tc.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", tc.Len())
	}
}

func TestTTLCache_PutTTL(t *testing.T) {
	t.Run("varying TTLs on same cache expire independently", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, time.Hour) // long default TTL
		tc.now = fc.Now

		tc.PutTTL("short", 1, 5*time.Millisecond)
		tc.PutTTL("long", 2, time.Hour)

		fc.Advance(15 * time.Millisecond)

		if v, ok := tc.Get("short"); ok || v != 0 {
			t.Fatalf("Get(short) = (%v, %v), want (0, false) after its short TTL elapsed", v, ok)
		}
		if v, ok := tc.Get("long"); !ok || v != 2 {
			t.Fatalf("Get(long) = (%v, %v), want (2, true); its own longer TTL should still hold", v, ok)
		}
	})

	t.Run("PutTTL overrides an existing entry's expiry", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, 5*time.Millisecond) // short default TTL
		tc.now = fc.Now
		tc.Put("a", 1) // uses default (short) TTL

		tc.PutTTL("a", 2, time.Hour) // override with a long TTL

		fc.Advance(15 * time.Millisecond)

		if v, ok := tc.Get("a"); !ok || v != 2 {
			t.Fatalf("Get(a) = (%v, %v), want (2, true); PutTTL's longer TTL should override the default", v, ok)
		}
	})

	t.Run("Put still uses the cache's default TTL", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, 5*time.Millisecond)
		tc.now = fc.Now
		tc.Put("a", 1)

		fc.Advance(15 * time.Millisecond)

		if v, ok := tc.Get("a"); ok || v != 0 {
			t.Fatalf("Get(a) = (%v, %v), want (0, false); Put should still expire after the default TTL", v, ok)
		}
	})

	t.Run("PutTTL marks entry most-recently-used and respects capacity", func(t *testing.T) {
		var mu sync.Mutex
		var evicted []string

		tc := NewTTL[string, int](2, time.Hour, WithOnEvict[string, int](func(k string, _ int) {
			mu.Lock()
			evicted = append(evicted, k)
			mu.Unlock()
		}))

		tc.PutTTL("a", 1, time.Hour)
		tc.PutTTL("b", 2, time.Minute)
		tc.Get("a")                    // a is now most-recently-used
		tc.PutTTL("c", 3, time.Second) // should evict b, the LRU entry

		if _, ok := tc.Get("b"); ok {
			t.Fatalf("expected b to be evicted by capacity")
		}

		mu.Lock()
		defer mu.Unlock()
		if len(evicted) != 1 || evicted[0] != "b" {
			t.Fatalf("evicted = %v, want [b]", evicted)
		}
	})
}

func TestTTLCache_ConcurrentAccess(_ *testing.T) {
	tc := NewTTL[int, int](64, 50*time.Millisecond, WithOnEvict[int, int](func(_, _ int) {}))

	const goroutines = 16
	const opsPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := (g*opsPerGoroutine + i) % 100
				tc.Put(key, i)
				tc.Get(key)
				if i%10 == 0 {
					tc.Delete(key)
				}
				tc.Len()
			}
		}(g)
	}
	wg.Wait()
}
