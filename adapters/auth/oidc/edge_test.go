package oidc_test

import (
	"sync"
	"testing"
	"time"
)

// TestEdgeVerify_concurrent proves the verifier is safe for concurrent use
// against one hermetic provider and a shared token.
func TestEdgeVerify_concurrent(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)
	ctx := t.Context()

	token := mintToken(t, idp.key, idp.kid, idp.srv.URL, "test-client", time.Now().Add(time.Hour), nil)

	var wg sync.WaitGroup

	errs := make(chan error, 8)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := a.Verify(ctx, token); err != nil {
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
