package jwt

import (
	"sync"
	"testing"
	"time"
)

// TestEdgeVerify_concurrent proves the adapter and its revocation store are
// safe for concurrent verification of a shared token.
func TestEdgeVerify_concurrent(t *testing.T) {
	t.Parallel()

	a := freshAdapter(t, baseOpts())
	ctx := t.Context()

	tok, err := a.Issue(ctx, "user-1", map[string]any{"role": "admin"}, time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	var wg sync.WaitGroup

	errs := make(chan error, 8)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := a.Verify(ctx, tok.Value); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Verify: %v", err)
	}
}
