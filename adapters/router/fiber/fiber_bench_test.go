package fiber

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/router"
)

// BenchmarkServeHTTP measures the Handle+ServeHTTP round trip, including
// param bridging, to track the per-request allocation cost of the
// adaptor/fasthttpadaptor wrapping in wrapHandler and ServeHTTP.
func BenchmarkServeHTTP(b *testing.B) {
	r, err := New(router.Options{})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	r.Handle("GET", "/hello/:name", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "name")))
	})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/hello/world", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)
	}
}

// BenchmarkServeHTTPAdaptor isolates the adaptor wrapping cost: a no-op
// handler removes handler logic from the measurement, so the ns/op and
// allocs/op reflect only the fasthttpadaptor conversion, the pooled recorder,
// and the header/status/body merge back into the fiber response.
func BenchmarkServeHTTPAdaptor(b *testing.B) {
	r, err := New(router.Options{})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	r.Handle("GET", "/hello/:name", func(_ http.ResponseWriter, _ *http.Request) {})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/hello/world", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)
	}
}

// BenchmarkServeHTTPParallel measures ServeHTTP under concurrent load. Each
// worker builds its own request/recorder pair (sharing one *http.Request
// across goroutines is unsafe), so this captures lock contention in the
// adaptor wrapping rather than harness races.
func BenchmarkServeHTTPParallel(b *testing.B) {
	r, err := New(router.Options{})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	r.Handle("GET", "/hello/:name", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "name")))
	})

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/hello/world", nil)
			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, req)
			if resp.Code != http.StatusOK {
				b.Error("ServeHTTP status =", resp.Code)
				return
			}
		}
	})
}
