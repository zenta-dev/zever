package session_test

import (
	"sync"
	"testing"
	"time"
)

// TestEdgeIssueVerify_concurrent proves the adapter is safe for concurrent
// issue and verify against a shared in-memory store.
func TestEdgeIssueVerify_concurrent(t *testing.T) {
	t.Parallel()

	a := newAdapter(t, newMemoryStore(t))
	ctx := t.Context()

	var wg sync.WaitGroup

	errs := make(chan error, 8)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			tok, err := a.Issue(ctx, "user-1", map[string]any{"role": "admin"}, time.Hour)
			if err != nil {
				errs <- err

				return
			}

			if _, err := a.Verify(ctx, tok.Value); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Issue/Verify: %v", err)
	}
}
