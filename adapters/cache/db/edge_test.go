package db

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestEdgeSet_nonPositiveTTLPersists covers the TTL boundary: a zero or
// negative per-call ttl means persist, so the entry survives a Get.
func TestEdgeSet_nonPositiveTTLPersists(t *testing.T) {
	t.Parallel()

	c := mustNew(t)
	ctx := t.Context()

	for _, ttl := range []time.Duration{0, -time.Second} {
		if err := c.Set(ctx, "k", []byte("v"), ttl); err != nil {
			t.Fatalf("Set(ttl=%v) = %v, want nil", ttl, err)
		}

		got, err := c.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get after Set(ttl=%v) = %v, want nil", ttl, err)
		}

		if string(got) != "v" {
			t.Fatalf("Get after Set(ttl=%v) = %q, want v", ttl, got)
		}
	}
}

// TestEdgeConcurrent_setGet exercises parallel Set/Get on distinct keys,
// proving the shared database pool is safe under concurrency.
func TestEdgeConcurrent_setGet(t *testing.T) {
	t.Parallel()

	c := mustNew(t)
	ctx := t.Context()

	var (
		wg   sync.WaitGroup
		n    atomic.Int64
		errs = make(chan error, 16)
	)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 4 {
				key := "k-" + strconv.FormatInt(n.Add(1), 10)

				if err := c.Set(ctx, key, []byte("v"), 0); err != nil {
					errs <- err

					return
				}

				if _, err := c.Get(ctx, key); err != nil {
					errs <- err

					return
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Set/Get: %v", err)
	}
}
