package stripe

import (
	"net/http/httptest"
	"sync"
	"testing"
)

// TestEdgeConcurrent proves the adapter and its HTTP client are safe for
// concurrent calls against one in-process API stub.
func TestEdgeConcurrent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(newDefaultMux(nil))
	t.Cleanup(srv.Close)

	b := openWithServer(t, srv)

	t.Cleanup(func() { _ = b.Close() })

	ctx := t.Context()

	var wg sync.WaitGroup

	errs := make(chan error, 8)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := b.CreateCustomer(ctx, "Ada", "ada@example.com", ""); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent CreateCustomer: %v", err)
	}
}
