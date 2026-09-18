package lrucache

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCache_GetPut(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, c *Cache[string, int])
	}{
		{
			name: "get on empty cache misses",
			run: func(t *testing.T, c *Cache[string, int]) {
				t.Helper()
				v, ok := c.Get("missing")
				if ok || v != 0 {
					t.Fatalf("Get(missing) = (%v, %v), want (0, false)", v, ok)
				}
			},
		},
		{
			name: "put then get hits",
			run: func(t *testing.T, c *Cache[string, int]) {
				t.Helper()
				c.Put("a", 1)
				v, ok := c.Get("a")
				if !ok || v != 1 {
					t.Fatalf("Get(a) = (%v, %v), want (1, true)", v, ok)
				}
			},
		},
		{
			name: "put updates existing key without growing len",
			run: func(t *testing.T, c *Cache[string, int]) {
				t.Helper()
				c.Put("a", 1)
				c.Put("a", 2)
				if c.Len() != 1 {
					t.Fatalf("Len() = %d, want 1", c.Len())
				}
				v, ok := c.Get("a")
				if !ok || v != 2 {
					t.Fatalf("Get(a) = (%v, %v), want (2, true)", v, ok)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New[string, int](4)
			tt.run(t, c)
		})
	}
}

func TestCache_Len(t *testing.T) {
	c := New[string, int](4)
	if c.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", c.Len())
	}
	c.Put("a", 1)
	c.Put("b", 2)
	if c.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", c.Len())
	}
}

func TestCache_EvictionOrder(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	// Access "a" so "b" becomes the least-recently-used entry.
	c.Get("a")
	c.Put("c", 3) // should evict "b", not "a"

	if _, ok := c.Get("b"); ok {
		t.Fatalf("expected b to be evicted")
	}
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("expected a to survive eviction, got (%v, %v)", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Fatalf("expected c to be present, got (%v, %v)", v, ok)
	}
	if c.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", c.Len())
	}
}

func TestCache_EvictionOrder_PutRefreshesRecency(t *testing.T) {
	c := New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	// Re-Put "a" (update, not insert) should also refresh recency.
	c.Put("a", 10)
	c.Put("c", 3) // should evict "b"

	if _, ok := c.Get("b"); ok {
		t.Fatalf("expected b to be evicted")
	}
	if v, ok := c.Get("a"); !ok || v != 10 {
		t.Fatalf("expected a to survive with updated value, got (%v, %v)", v, ok)
	}
}

func TestCache_OnEvict(t *testing.T) {
	var mu sync.Mutex
	var evicted []string

	c := New[string, int](2, WithOnEvict[string, int](func(k string, v int) {
		mu.Lock()
		evicted = append(evicted, fmt.Sprintf("%s=%d", k, v))
		mu.Unlock()
	}))

	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // evicts a=1 (least recently used)

	mu.Lock()
	defer mu.Unlock()
	if len(evicted) != 1 || evicted[0] != "a=1" {
		t.Fatalf("evicted = %v, want [a=1]", evicted)
	}
}

func TestCache_OnEvict_NotFiredWithinCapacity(t *testing.T) {
	fired := false
	c := New[string, int](4, WithOnEvict[string, int](func(_ string, _ int) {
		fired = true
	}))

	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("a", 3) // update, no eviction

	if fired {
		t.Fatalf("OnEvict should not fire when under capacity")
	}
}

func TestCache_Delete(t *testing.T) {
	t.Run("delete present key returns value", func(t *testing.T) {
		c := New[string, int](4)
		c.Put("a", 1)
		v, ok := c.Delete("a")
		if !ok || v != 1 {
			t.Fatalf("Delete(a) = (%v, %v), want (1, true)", v, ok)
		}
		if _, ok := c.Get("a"); ok {
			t.Fatalf("expected a to be gone after Delete")
		}
		if c.Len() != 0 {
			t.Fatalf("Len() = %d, want 0", c.Len())
		}
	})

	t.Run("delete missing key reports absent", func(t *testing.T) {
		c := New[string, int](4)
		v, ok := c.Delete("missing")
		if ok || v != 0 {
			t.Fatalf("Delete(missing) = (%v, %v), want (0, false)", v, ok)
		}
	})

	t.Run("delete does not invoke OnEvict", func(t *testing.T) {
		fired := false
		c := New[string, int](4, WithOnEvict[string, int](func(_ string, _ int) {
			fired = true
		}))
		c.Put("a", 1)
		c.Delete("a")
		if fired {
			t.Fatalf("explicit Delete must not invoke OnEvict")
		}
	})
}

func TestNew_NonPositiveCapacityUsesDefault(t *testing.T) {
	for _, capacity := range []int{0, -1, -100} {
		c := New[int, int](capacity)
		for i := 0; i < defaultCapacity+10; i++ {
			c.Put(i, i)
		}
		if c.Len() != defaultCapacity {
			t.Fatalf("capacity %d: Len() = %d, want %d", capacity, c.Len(), defaultCapacity)
		}
	}
}

func TestCache_GetOrCompute(t *testing.T) {
	t.Run("absent key computes and stores", func(t *testing.T) {
		c := New[string, int](4)
		calls := 0
		v, ok := c.GetOrCompute("a", func() int {
			calls++
			return 42
		})
		if ok {
			t.Fatalf("GetOrCompute(a) ok = true, want false for absent key")
		}
		if v != 42 {
			t.Fatalf("GetOrCompute(a) = %v, want 42", v)
		}
		if calls != 1 {
			t.Fatalf("compute called %d times, want 1", calls)
		}
		if got, ok := c.Get("a"); !ok || got != 42 {
			t.Fatalf("Get(a) after GetOrCompute = (%v, %v), want (42, true)", got, ok)
		}
	})

	t.Run("present key returns existing value without computing", func(t *testing.T) {
		c := New[string, int](4)
		c.Put("a", 1)

		called := false
		v, ok := c.GetOrCompute("a", func() int {
			called = true
			return 999
		})
		if !ok {
			t.Fatalf("GetOrCompute(a) ok = false, want true for present key")
		}
		if v != 1 {
			t.Fatalf("GetOrCompute(a) = %v, want 1 (existing value)", v)
		}
		if called {
			t.Fatalf("compute must not be called when key is present")
		}
	})

	t.Run("subject to capacity eviction and fires OnEvict", func(t *testing.T) {
		var mu sync.Mutex
		var evicted []string

		c := New[string, int](2, WithOnEvict[string, int](func(k string, v int) {
			mu.Lock()
			evicted = append(evicted, fmt.Sprintf("%s=%d", k, v))
			mu.Unlock()
		}))

		c.Put("a", 1)
		c.Put("b", 2)
		c.GetOrCompute("c", func() int { return 3 }) // should evict a (LRU)

		mu.Lock()
		defer mu.Unlock()
		if len(evicted) != 1 || evicted[0] != "a=1" {
			t.Fatalf("evicted = %v, want [a=1]", evicted)
		}
	})

	t.Run("moves existing key to front of recency list", func(t *testing.T) {
		c := New[string, int](2)
		c.Put("a", 1)
		c.Put("b", 2)
		c.GetOrCompute("a", func() int { return 999 }) // touches a, b becomes LRU
		c.Put("c", 3)                                  // should evict b, not a

		if _, ok := c.Get("b"); ok {
			t.Fatalf("expected b to be evicted")
		}
		if v, ok := c.Get("a"); !ok || v != 1 {
			t.Fatalf("expected a to survive, got (%v, %v)", v, ok)
		}
	})
}

// TestCache_GetOrCompute_ConcurrentSingleCompute races many goroutines
// through GetOrCompute on the same absent key and asserts compute was
// called exactly once and every goroutine observed the same resulting
// value, proving the check-and-insert is atomic under concurrent access.
// Run with -race to exercise the actual data race the atomicity guarantee
// protects against.
func TestCache_GetOrCompute_ConcurrentSingleCompute(t *testing.T) {
	c := New[string, int](4)

	var computeCalls int32
	var wg sync.WaitGroup

	const goroutines = 64
	results := make([]int, goroutines)

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			v, _ := c.GetOrCompute("shared", func() int {
				atomic.AddInt32(&computeCalls, 1)
				return 1234
			})
			results[g] = v
		}(g)
	}
	wg.Wait()

	if got := atomic.LoadInt32(&computeCalls); got != 1 {
		t.Fatalf("compute called %d times, want exactly 1", got)
	}
	for i, v := range results {
		if v != 1234 {
			t.Fatalf("goroutine %d observed value %v, want 1234", i, v)
		}
	}
	if c.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", c.Len())
	}
}

func TestCache_Clear(t *testing.T) {
	t.Run("clear with nil callback empties the cache", func(t *testing.T) {
		c := New[string, int](4)
		c.Put("a", 1)
		c.Put("b", 2)

		c.Clear(nil)

		if c.Len() != 0 {
			t.Fatalf("Len() = %d, want 0 after Clear", c.Len())
		}
		if _, ok := c.Get("a"); ok {
			t.Fatalf("expected a to be gone after Clear")
		}
		if _, ok := c.Get("b"); ok {
			t.Fatalf("expected b to be gone after Clear")
		}
	})

	t.Run("clear with callback invokes it once per entry", func(t *testing.T) {
		c := New[string, int](4)
		c.Put("a", 1)
		c.Put("b", 2)
		c.Put("c", 3)

		var mu sync.Mutex
		got := map[string]int{}
		c.Clear(func(k string, v int) {
			mu.Lock()
			got[k] = v
			mu.Unlock()
		})

		want := map[string]int{"a": 1, "b": 2, "c": 3}
		if len(got) != len(want) {
			t.Fatalf("Clear callback fired for %v, want %v", got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("Clear callback got %s=%v, want %s=%v", k, got[k], k, v)
			}
		}
		if c.Len() != 0 {
			t.Fatalf("Len() = %d, want 0 after Clear", c.Len())
		}
	})

	t.Run("clear on empty cache with callback fires nothing", func(t *testing.T) {
		c := New[string, int](4)
		fired := false
		c.Clear(func(_ string, _ int) { fired = true })
		if fired {
			t.Fatalf("Clear callback must not fire on an empty cache")
		}
	})

	t.Run("clear does not invoke OnEvict", func(t *testing.T) {
		onEvictFired := false
		c := New[string, int](4, WithOnEvict[string, int](func(_ string, _ int) {
			onEvictFired = true
		}))
		c.Put("a", 1)
		c.Put("b", 2)

		clearFired := 0
		c.Clear(func(_ string, _ int) { clearFired++ })

		if onEvictFired {
			t.Fatalf("Clear must not invoke OnEvict")
		}
		if clearFired != 2 {
			t.Fatalf("Clear callback fired %d times, want 2", clearFired)
		}
	})

	t.Run("cache is usable after Clear", func(t *testing.T) {
		c := New[string, int](2)
		c.Put("a", 1)
		c.Clear(nil)

		c.Put("x", 10)
		c.Put("y", 20)
		c.Put("z", 30) // over capacity, should evict x

		if _, ok := c.Get("x"); ok {
			t.Fatalf("expected x to be evicted after refill past capacity")
		}
		if v, ok := c.Get("y"); !ok || v != 20 {
			t.Fatalf("Get(y) = (%v, %v), want (20, true)", v, ok)
		}
		if v, ok := c.Get("z"); !ok || v != 30 {
			t.Fatalf("Get(z) = (%v, %v), want (30, true)", v, ok)
		}
	})
}

func TestCache_ConcurrentAccess(_ *testing.T) {
	c := New[int, int](64, WithOnEvict[int, int](func(_, _ int) {}))

	const goroutines = 16
	const opsPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := (g*opsPerGoroutine + i) % 100
				c.Put(key, i)
				c.Get(key)
				if i%10 == 0 {
					c.Delete(key)
				}
				c.Len()
			}
		}(g)
	}
	wg.Wait()
}
