package oidc_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/auth"
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

// TestEdgeVerify_canceledContext proves an already-canceled caller context
// fails closed without reaching the verifier.
func TestEdgeVerify_canceledContext(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	raw := mintToken(t, idp.key, idp.kid, idp.srv.URL, "test-client", time.Now().Add(time.Hour), nil)

	_, err := a.Verify(ctx, raw)
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("Verify(canceled) err = %v, want context canceled", err)
	}
	if !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatalf("Verify(canceled) err = %v, want wrap ErrInvalidToken", err)
	}
}

// TestEdgeClose_idempotent proves Close is safe to call repeatedly.
func TestEdgeClose_idempotent(t *testing.T) {
	t.Parallel()

	idp := newFakeIDP(t)
	a := newAdapter(t, idp)

	if err := a.Close(); err != nil {
		t.Fatalf("Close #1 err = %v, want nil", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close #2 err = %v, want nil", err)
	}
}
