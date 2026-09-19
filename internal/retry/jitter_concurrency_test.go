package retry

import (
	"sync"
	"testing"
	"time"
)

// TestRandFloat64ConcurrentCallsDontCollide is the regression test for the
// fixed thundering-herd bug: randFloat64 used to seed a fresh PRNG from
// time.Now().UnixNano() on every call, so concurrent callers landing in the
// same nanosecond could get identical jitter. math/rand/v2's top-level
// Float64 is concurrency-safe and doesn't reseed per call, so a burst of
// concurrent calls should not all agree on the same value.
func TestRandFloat64ConcurrentCallsDontCollide(t *testing.T) {
	t.Parallel()

	const workers = 200

	results := make([]float64, workers)

	var wg sync.WaitGroup

	start := make(chan struct{})

	for i := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			results[i] = randFloat64()
		}()
	}

	close(start)
	wg.Wait()

	seen := make(map[float64]int, workers)
	for _, v := range results {
		if v < 0 || v >= 1 {
			t.Fatalf("randFloat64() = %v, want in [0, 1)", v)
		}

		seen[v]++
	}

	// A handful of exact collisions among 200 concurrent float64 draws
	// would be an astronomically unlikely coincidence with a real PRNG;
	// treat any collision as evidence the old per-call-reseed bug is back.
	if len(seen) != workers {
		t.Errorf("got %d unique values from %d concurrent calls, want %d (collisions indicate correlated/reseeded jitter)", len(seen), workers, workers)
	}
}

// TestPolicyNextDelayConcurrentJitterVaries exercises the same regression
// through the public Policy.NextDelay path used by callers like
// queue/redis's poll backoff.
func TestPolicyNextDelayConcurrentJitterVaries(t *testing.T) {
	t.Parallel()

	p := Policy{BaseDelay: 10 * time.Millisecond, Multiplier: 2, MaxDelay: 200 * time.Millisecond, Jitter: 0.5, JitterMode: JitterAdditive}

	const workers = 200

	results := make([]time.Duration, workers)

	var wg sync.WaitGroup

	start := make(chan struct{})

	for i := range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			results[i] = p.NextDelay(3)
		}()
	}

	close(start)
	wg.Wait()

	seen := make(map[time.Duration]int, workers)
	for _, v := range results {
		seen[v]++
	}

	if len(seen) < workers/2 {
		t.Errorf("got only %d unique delays from %d concurrent NextDelay calls, want most of them distinct (suspiciously correlated jitter)", len(seen), workers)
	}
}
