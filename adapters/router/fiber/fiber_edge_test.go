package fiber

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/router"
)

func TestServeHTTP_concurrentSafe(t *testing.T) {
	t.Parallel()

	r, err := New(router.Options{})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	r.Handle("GET", "/conc/{id}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "id")))
	})

	const workers = 32

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/conc/9", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || rec.Body.String() != "9" {
				t.Errorf("ServeHTTP() = %d %q, want 200 9", rec.Code, rec.Body.String())
			}
		}()
	}
	wg.Wait()
}

// TestEdgeRegister_resolvesAdapter covers the exported Register wiring: after
// Register the router resolves through the battery registry, and a repeated
// Register is tolerated (the duplicate error is discarded) without changing
// resolution.
func TestEdgeRegister_resolvesAdapter(t *testing.T) {
	// Serial: Register mutates the process-global battery registry.
	Register()
	Register()

	r, err := router.Open(router.AdapterFiber, router.Options{})
	if err != nil {
		t.Fatalf("router.Open() after Register = %v, want nil", err)
	}

	if r == nil {
		t.Fatal("router.Open() after Register = nil, want router")
	}
}
