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
