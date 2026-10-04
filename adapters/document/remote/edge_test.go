package remote

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/document"
)

// TestEdgeRender_concurrent proves many goroutines can Render through one
// adapter against a shared in-process service.
func TestEdgeRender_concurrent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 ok"))
	}))
	t.Cleanup(srv.Close)

	d, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := t.Context()

	var wg sync.WaitGroup

	errs := make(chan error, 8)

	for range 8 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := d.Render(ctx, []byte("<html></html>"), document.FormatPDF); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Render: %v", err)
	}
}

// TestEdgeRender_nilSource covers the nil-source boundary: an empty body is
// still a well-formed request and must not panic.
func TestEdgeRender_nilSource(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.4 ok"))
	}))
	t.Cleanup(srv.Close)

	d, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := d.Render(t.Context(), nil, document.FormatPDF); err != nil {
		t.Fatalf("Render(nil) = %v, want nil", err)
	}
}
