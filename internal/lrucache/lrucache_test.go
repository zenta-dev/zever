package lrucache

import (
	"fmt"
	"sync"
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
				v, ok := c.Get("missing")
				if ok || v != 0 {
					t.Fatalf("Get(missing) = (%v, %v), want (0, false)", v, ok)
				}
			},
		},
		{
			name: "put then get hits",
			run: func(t *testing.T, c *Cache[string, int]) {
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
	c := New[string, int](4, WithOnEvict[string, int](func(k string, v int) {
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
		c := New[string, int](4, WithOnEvict[string, int](func(k string, v int) {
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

func TestCache_ConcurrentAccess(t *testing.T) {
	c := New[int, int](64, WithOnEvict[int, int](func(k, v int) {}))

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
