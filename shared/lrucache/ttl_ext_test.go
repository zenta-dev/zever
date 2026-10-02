package lrucache

import (
	"bytes"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func equalBytes(a, b []byte) bool { return bytes.Equal(a, b) }

func TestTTLCache_PersistSentinel(t *testing.T) {
	fc := newFakeClock()
	tc := NewTTL[string, int](4, time.Hour)
	tc.now = fc.Now

	tc.PutTTL("zero", 1, 0)
	tc.PutTTL("neg", 2, -time.Minute)
	fc.Advance(48 * time.Hour)
	if v, ok := tc.Get("zero"); !ok || v != 1 {
		t.Fatalf("Get(zero) = (%v, %v), want (1, true) persist", v, ok)
	}
	if v, ok := tc.Get("neg"); !ok || v != 2 {
		t.Fatalf("Get(neg) = (%v, %v), want (2, true) persist", v, ok)
	}
	if _, exp, ok := tc.GetWithExpiry("zero"); !ok || !exp.IsZero() {
		t.Fatalf("GetWithExpiry(zero) = (_, %v, %v), want zero expiry, true", exp, ok)
	}

	// Default TTL <= 0 means Put persists too.
	fc2 := newFakeClock()
	tc2 := NewTTL[string, int](4, 0)
	tc2.now = fc2.Now
	tc2.Put("a", 1)
	fc2.Advance(48 * time.Hour)
	if v, ok := tc2.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = (%v, %v), want (1, true) persist default", v, ok)
	}
}

func TestTTLCache_GetWithExpiry(t *testing.T) {
	t.Run("returns value and absolute expiry", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, time.Hour)
		tc.now = fc.Now
		wantExp := fc.Now().Add(time.Hour)
		tc.Put("a", 1)
		v, exp, ok := tc.GetWithExpiry("a")
		if !ok || v != 1 {
			t.Fatalf("GetWithExpiry(a) = (%v, _, %v), want (1, _, true)", v, ok)
		}
		if !exp.Equal(wantExp) {
			t.Fatalf("GetWithExpiry(a) exp = %v, want %v", exp, wantExp)
		}
	})

	t.Run("missing reports absent", func(t *testing.T) {
		tc := NewTTL[string, int](4, time.Hour)
		if _, _, ok := tc.GetWithExpiry("missing"); ok {
			t.Fatal("GetWithExpiry(missing) ok = true, want false")
		}
	})

	t.Run("expired purges and reports absent", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, time.Millisecond)
		tc.now = fc.Now
		tc.Put("a", 1)
		fc.Advance(5 * time.Millisecond)
		if _, _, ok := tc.GetWithExpiry("a"); ok {
			t.Fatal("GetWithExpiry(expired) ok = true, want false")
		}
		if tc.Len() != 0 {
			t.Fatalf("Len() = %d, want 0 after lazy purge", tc.Len())
		}
	})

	t.Run("hit refreshes recency", func(t *testing.T) {
		tc := NewTTL[string, int](2, time.Hour)
		tc.Put("a", 1)
		tc.Put("b", 2)
		if _, _, ok := tc.GetWithExpiry("a"); !ok {
			t.Fatal("GetWithExpiry(a) miss, want hit")
		}
		tc.Put("c", 3) // should evict b, not a
		if _, _, ok := tc.GetWithExpiry("b"); ok {
			t.Fatal("expected b evicted")
		}
		if _, _, ok := tc.GetWithExpiry("a"); !ok {
			t.Fatal("expected a retained")
		}
	})
}

func TestTTLCache_ExpiryBoundaryExactDeadline(t *testing.T) {
	fc := newFakeClock()
	tc := NewTTL[string, int](4, 10*time.Millisecond)
	tc.now = fc.Now
	tc.Put("a", 1)
	fc.Advance(10 * time.Millisecond) // exactly at deadline
	if _, ok := tc.Get("a"); ok {
		t.Fatal("Get(a) at exact deadline = hit, want expired (!After boundary)")
	}
}

func TestTTLCache_SetIfAbsent(t *testing.T) {
	t.Run("absent stores with ttl", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, time.Hour)
		tc.now = fc.Now
		if !tc.SetIfAbsent("a", 1, 5*time.Millisecond) {
			t.Fatal("SetIfAbsent(absent) = false, want true")
		}
		fc.Advance(15 * time.Millisecond)
		if _, ok := tc.Get("a"); ok {
			t.Fatal("expected per-call TTL to expire entry")
		}
	})

	t.Run("live present refuses without overwrite", func(t *testing.T) {
		tc := NewTTL[string, int](4, time.Hour)
		tc.Put("a", 1)
		if tc.SetIfAbsent("a", 99, time.Hour) {
			t.Fatal("SetIfAbsent(live) = true, want false")
		}
		if v, _ := tc.Get("a"); v != 1 {
			t.Fatalf("stored value = %v, want 1 untouched", v)
		}
	})

	t.Run("failed SetIfAbsent does not refresh recency", func(t *testing.T) {
		tc := NewTTL[string, int](2, time.Hour)
		tc.Put("a", 1)
		tc.Put("b", 2)
		if tc.SetIfAbsent("a", 99, time.Hour) {
			t.Fatal("SetIfAbsent(live) = true, want false")
		}
		// a was NOT promoted by the failed SetIfAbsent, so a is still LRU:
		// inserting c must evict a.
		tc.Put("c", 3)
		if _, ok := tc.Get("a"); ok {
			t.Fatal("expected a evicted (failed SetIfAbsent must not promote)")
		}
		if v, ok := tc.Get("b"); !ok || v != 2 {
			t.Fatalf("Get(b) = (%v, %v), want (2, true) retained", v, ok)
		}
	})

	t.Run("expired present stores", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, time.Millisecond)
		tc.now = fc.Now
		tc.Put("a", 1)
		fc.Advance(5 * time.Millisecond)
		if !tc.SetIfAbsent("a", 2, time.Hour) {
			t.Fatal("SetIfAbsent(expired) = false, want true")
		}
		if v, ok := tc.Get("a"); !ok || v != 2 {
			t.Fatalf("Get(a) = (%v, %v), want (2, true)", v, ok)
		}
	})

	t.Run("non-positive ttl persists", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, time.Hour)
		tc.now = fc.Now
		if !tc.SetIfAbsent("a", 1, 0) {
			t.Fatal("SetIfAbsent = false, want true")
		}
		fc.Advance(48 * time.Hour)
		if v, ok := tc.Get("a"); !ok || v != 1 {
			t.Fatalf("Get(a) = (%v, %v), want persisted", v, ok)
		}
	})

	t.Run("insert evicts LRU and fires OnEvict unwrapped", func(t *testing.T) {
		var mu sync.Mutex
		var evicted []string
		tc := NewTTL[string, int](2, time.Hour, WithOnEvict[string, int](func(k string, _ int) {
			mu.Lock()
			evicted = append(evicted, k)
			mu.Unlock()
		}))
		tc.Put("a", 1)
		tc.Put("b", 2)
		if !tc.SetIfAbsent("c", 3, time.Hour) {
			t.Fatal("SetIfAbsent = false, want true")
		}
		mu.Lock()
		defer mu.Unlock()
		if len(evicted) != 1 || evicted[0] != "a" {
			t.Fatalf("evicted = %v, want [a]", evicted)
		}
	})
}

func TestTTLCache_SetIfAbsent_ConcurrentSingleWinner(t *testing.T) {
	tc := NewTTL[string, int](4, time.Hour)
	var wins int32
	var wg sync.WaitGroup
	const n = 32
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			if tc.SetIfAbsent("k", 1, time.Hour) {
				atomic.AddInt32(&wins, 1)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&wins); got != 1 {
		t.Fatalf("winners = %d, want exactly 1", got)
	}
	if v, ok := tc.Get("k"); !ok || v != 1 {
		t.Fatalf("Get(k) = (%v, %v), want (1, true)", v, ok)
	}
}

func TestTTLCache_CompareAndDelete(t *testing.T) {
	t.Run("match deletes", func(t *testing.T) {
		tc := NewTTL[string, []byte](4, time.Hour)
		tc.Put("k", []byte("holder1"))
		if !tc.CompareAndDelete("k", []byte("holder1"), equalBytes) {
			t.Fatal("CompareAndDelete(match) = false, want true")
		}
		if _, ok := tc.Get("k"); ok {
			t.Fatal("expected k gone")
		}
	})

	t.Run("mismatch keeps", func(t *testing.T) {
		tc := NewTTL[string, []byte](4, time.Hour)
		tc.Put("k", []byte("holder1"))
		if tc.CompareAndDelete("k", []byte("rival"), equalBytes) {
			t.Fatal("CompareAndDelete(mismatch) = true, want false")
		}
		if v, ok := tc.Get("k"); !ok || string(v) != "holder1" {
			t.Fatalf("Get(k) = (%q, %v), want holder1 retained", v, ok)
		}
	})

	t.Run("missing false", func(t *testing.T) {
		tc := NewTTL[string, []byte](4, time.Hour)
		if tc.CompareAndDelete("missing", []byte("x"), equalBytes) {
			t.Fatal("CompareAndDelete(missing) = true, want false")
		}
	})

	t.Run("expired purges and false", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, []byte](4, time.Millisecond)
		tc.now = fc.Now
		tc.Put("k", []byte("v"))
		fc.Advance(5 * time.Millisecond)
		if tc.CompareAndDelete("k", []byte("v"), equalBytes) {
			t.Fatal("CompareAndDelete(expired) = true, want false")
		}
		if tc.Len() != 0 {
			t.Fatalf("Len() = %d, want 0 after purge", tc.Len())
		}
	})

	t.Run("nil equal never matches", func(t *testing.T) {
		tc := NewTTL[string, []byte](4, time.Hour)
		tc.Put("k", []byte("v"))
		if tc.CompareAndDelete("k", []byte("v"), nil) {
			t.Fatal("CompareAndDelete(nil equal) = true, want false")
		}
	})
}

func TestTTLCache_CompareAndSwap(t *testing.T) {
	t.Run("match swaps preserving expiry", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, []byte](4, time.Hour)
		tc.now = fc.Now
		tc.Put("k", []byte("1"))
		_, expBefore, ok := tc.GetWithExpiry("k")
		if !ok {
			t.Fatal("GetWithExpiry miss, want hit")
		}
		if !tc.CompareAndSwap("k", []byte("1"), []byte("2"), equalBytes) {
			t.Fatal("CompareAndSwap(match) = false, want true")
		}
		v, expAfter, ok := tc.GetWithExpiry("k")
		if !ok || string(v) != "2" {
			t.Fatalf("GetWithExpiry = (%q, %v), want 2", v, ok)
		}
		if !expAfter.Equal(expBefore) {
			t.Fatalf("expiry moved %v -> %v, want preserved", expBefore, expAfter)
		}
	})

	t.Run("mismatch missing expired nil-equal false", func(t *testing.T) {
		tc := NewTTL[string, []byte](4, time.Hour)
		tc.Put("k", []byte("v"))
		if tc.CompareAndSwap("k", []byte("other"), []byte("n"), equalBytes) {
			t.Error("mismatch = true, want false")
		}
		if tc.CompareAndSwap("missing", []byte("v"), []byte("n"), equalBytes) {
			t.Error("missing = true, want false")
		}
		if tc.CompareAndSwap("k", []byte("v"), []byte("n"), nil) {
			t.Error("nil equal = true, want false")
		}
		if v, ok := tc.Get("k"); !ok || string(v) != "v" {
			t.Errorf("Get(k) = (%q, %v), want v untouched", v, ok)
		}

		fc := newFakeClock()
		tc2 := NewTTL[string, []byte](4, time.Millisecond)
		tc2.now = fc.Now
		tc2.Put("e", []byte("v"))
		fc.Advance(5 * time.Millisecond)
		if tc2.CompareAndSwap("e", []byte("v"), []byte("n"), equalBytes) {
			t.Error("expired = true, want false")
		}
	})
}

func TestTTLCache_CompareAndExtend(t *testing.T) {
	t.Run("match renews ttl", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, []byte](4, 20*time.Millisecond)
		tc.now = fc.Now
		tc.Put("k", []byte("holder1"))
		_, expBefore, _ := tc.GetWithExpiry("k")
		fc.Advance(10 * time.Millisecond)
		if !tc.CompareAndExtend("k", []byte("holder1"), 5*time.Minute, equalBytes) {
			t.Fatal("CompareAndExtend(match) = false, want true")
		}
		_, expAfter, ok := tc.GetWithExpiry("k")
		if !ok {
			t.Fatal("GetWithExpiry miss after extend, want hit")
		}
		if !expAfter.After(expBefore) {
			t.Fatalf("expiry not renewed: before %v after %v", expBefore, expAfter)
		}
		fc.Advance(25 * time.Millisecond) // past original 20ms TTL
		if _, ok := tc.Get("k"); !ok {
			t.Fatal("Get(k) after original TTL = miss, want hit (extended)")
		}
	})

	t.Run("non-positive ttl clears expiry", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, []byte](4, 20*time.Millisecond)
		tc.now = fc.Now
		tc.Put("k", []byte("holder1"))
		if !tc.CompareAndExtend("k", []byte("holder1"), 0, equalBytes) {
			t.Fatal("CompareAndExtend(persist) = false, want true")
		}
		if _, exp, ok := tc.GetWithExpiry("k"); !ok || !exp.IsZero() {
			t.Fatalf("GetWithExpiry = (_, %v, %v), want zero expiry persist", exp, ok)
		}
		fc.Advance(time.Hour)
		if _, ok := tc.Get("k"); !ok {
			t.Fatal("Get(k) after clear = miss, want persisted")
		}
	})

	t.Run("mismatch missing expired nil-equal false", func(t *testing.T) {
		tc := NewTTL[string, []byte](4, time.Hour)
		tc.Put("k", []byte("v"))
		if tc.CompareAndExtend("k", []byte("other"), time.Minute, equalBytes) {
			t.Error("mismatch = true, want false")
		}
		if tc.CompareAndExtend("missing", []byte("x"), time.Minute, equalBytes) {
			t.Error("missing = true, want false")
		}
		if tc.CompareAndExtend("k", []byte("v"), time.Minute, nil) {
			t.Error("nil equal = true, want false")
		}

		fc := newFakeClock()
		tc2 := NewTTL[string, []byte](4, time.Millisecond)
		tc2.now = fc.Now
		tc2.Put("e", []byte("v"))
		fc.Advance(5 * time.Millisecond)
		if tc2.CompareAndExtend("e", []byte("v"), time.Minute, equalBytes) {
			t.Error("expired = true, want false")
		}
		if tc2.Len() != 0 {
			t.Errorf("Len() = %d, want 0 after purge", tc2.Len())
		}
	})
}

func TestTTLCache_PurgeExpired(t *testing.T) {
	fc := newFakeClock()
	tc := NewTTL[string, int](4, 10*time.Millisecond)
	tc.now = fc.Now
	tc.Put("short", 1)
	tc.PutTTL("long", 2, time.Hour)
	tc.PutTTL("persist", 3, 0)
	fc.Advance(20 * time.Millisecond)
	if n := tc.PurgeExpired(); n != 1 {
		t.Fatalf("PurgeExpired() = %d, want 1", n)
	}
	if _, ok := tc.Get("short"); ok {
		t.Error("expected short purged")
	}
	if v, ok := tc.Get("long"); !ok || v != 2 {
		t.Errorf("Get(long) = (%v, %v), want (2, true)", v, ok)
	}
	if v, ok := tc.Get("persist"); !ok || v != 3 {
		t.Errorf("Get(persist) = (%v, %v), want (3, true)", v, ok)
	}
	if n := tc.PurgeExpired(); n != 0 {
		t.Fatalf("PurgeExpired() again = %d, want 0", n)
	}
}

func TestTTLCache_Range(t *testing.T) {
	t.Run("visits snapshot with expiries and stops early", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, int](4, time.Hour)
		tc.now = fc.Now
		tc.Put("a", 1)
		tc.PutTTL("b", 2, 0) // persist
		seen := map[string]time.Time{}
		tc.Range(func(k string, exp time.Time) bool {
			seen[k] = exp
			return true
		})
		if len(seen) != 2 {
			t.Fatalf("Range visited %v, want 2 entries", seen)
		}
		if !seen["b"].IsZero() {
			t.Errorf("persist expiry = %v, want zero", seen["b"])
		}
		if seen["a"].IsZero() {
			t.Error("timed expiry is zero, want non-zero")
		}

		count := 0
		tc.Range(func(_ string, _ time.Time) bool {
			count++
			return false // stop after first
		})
		if count != 1 {
			t.Fatalf("early-stop Range visited %d, want 1", count)
		}
	})

	t.Run("does not affect recency and tolerates callback cache access", func(t *testing.T) {
		tc := NewTTL[string, int](2, time.Hour)
		tc.Put("a", 1)
		tc.Put("b", 2)
		tc.Range(func(_ string, _ time.Time) bool {
			_, _ = tc.Get("a") // safe: Range releases lock before callback
			return true
		})
		// Get(a) inside the callback promoted a, so b is LRU.
		tc.Put("c", 3)
		if _, ok := tc.Get("b"); ok {
			t.Fatal("expected b evicted after callback promoted a")
		}
	})
}

// TestTTLCache_CounterPattern proves a cache adapter can implement
// core/cache Increment/Decrement (parse, add delta, preserve expiry) using
// only the new API, with no parallel side structure. addDelta mirrors the
// memory adapter's semantics: missing or expired restarts, persist entries
// stay persist, timed entries keep their absolute expiry.
func TestTTLCache_CounterPattern(t *testing.T) {
	addDelta := func(tc *TTLCache[string, []byte], key string, delta int64) error {
		for {
			cur, _, ok := tc.GetWithExpiry(key)
			if !ok {
				if tc.SetIfAbsent(key, []byte(strconv.FormatInt(delta, 10)), 0) {
					return nil
				}
				continue
			}
			n, err := strconv.ParseInt(string(cur), 10, 64)
			if err != nil {
				return err
			}
			next := []byte(strconv.FormatInt(n+delta, 10))
			if tc.CompareAndSwap(key, cur, next, equalBytes) {
				return nil
			}
		}
	}

	t.Run("missing starts at delta and preserves expiry on increment", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, []byte](4, time.Hour)
		tc.now = fc.Now
		if err := addDelta(tc, "n", 1); err != nil {
			t.Fatalf("addDelta() error = %v", err)
		}
		if err := addDelta(tc, "n", 1); err != nil {
			t.Fatalf("addDelta() error = %v", err)
		}
		v, exp, ok := tc.GetWithExpiry("n")
		if !ok || string(v) != "2" {
			t.Fatalf("GetWithExpiry(n) = (%q, %v), want 2 persist", v, ok)
		}
		if !exp.IsZero() {
			t.Fatalf("expiry = %v, want zero persist", exp)
		}

		// Timed base keeps its absolute expiry across increments.
		tc.PutTTL("t", []byte("41"), time.Hour)
		_, expBefore, _ := tc.GetWithExpiry("t")
		if err := addDelta(tc, "t", 1); err != nil {
			t.Fatalf("addDelta() error = %v", err)
		}
		v, expAfter, _ := tc.GetWithExpiry("t")
		if string(v) != "42" {
			t.Fatalf("GetWithExpiry(t) = %q, want 42", v)
		}
		if !expAfter.Equal(expBefore) {
			t.Fatalf("expiry moved %v -> %v, want preserved", expBefore, expAfter)
		}
	})

	t.Run("expired restarts", func(t *testing.T) {
		fc := newFakeClock()
		tc := NewTTL[string, []byte](4, time.Millisecond)
		tc.now = fc.Now
		tc.Put("n", []byte("41"))
		fc.Advance(5 * time.Millisecond)
		if err := addDelta(tc, "n", 1); err != nil {
			t.Fatalf("addDelta() error = %v", err)
		}
		if v, ok := tc.Get("n"); !ok || string(v) != "1" {
			t.Fatalf("Get(n) = (%q, %v), want 1 restarted", v, ok)
		}
	})

	t.Run("concurrent increments sum exactly once per op", func(t *testing.T) {
		tc := NewTTL[string, []byte](64, time.Hour)
		var wg sync.WaitGroup
		const workers = 8
		const perWorker = 50
		wg.Add(workers)
		for range workers {
			go func() {
				defer wg.Done()
				for range perWorker {
					for {
						cur, _, ok := tc.GetWithExpiry("counter")
						if !ok {
							if tc.SetIfAbsent("counter", []byte("1"), 0) {
								break
							}
							continue
						}
						n, err := strconv.ParseInt(string(cur), 10, 64)
						if err != nil {
							t.Errorf("parse error = %v", err)
							return
						}
						if tc.CompareAndSwap("counter", cur, []byte(strconv.FormatInt(n+1, 10)), equalBytes) {
							break
						}
					}
				}
			}()
		}
		wg.Wait()
		if v, ok := tc.Get("counter"); !ok || string(v) != "400" {
			t.Fatalf("Get(counter) = (%q, %v), want 400", v, ok)
		}
	})
}
