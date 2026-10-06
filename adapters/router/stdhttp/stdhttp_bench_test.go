package stdhttp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/router"
)

func benchRouter(b *testing.B) router.Router {
	b.Helper()
	r, err := New(router.Options{})
	if err != nil {
		b.Fatalf("New() = %v", err)
	}
	return r
}

// BenchmarkServeHTTPParam measures the ServeHTTP round trip with param
// bridging.
func BenchmarkServeHTTPParam(b *testing.B) {
	r := benchRouter(b)
	r.Handle("GET", "/hello/{name}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "name")))
	})
	req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/hello/world", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}

// BenchmarkServeHTTPStatic measures the no-param fast path.
func BenchmarkServeHTTPStatic(b *testing.B) {
	r := benchRouter(b)
	r.Handle("GET", "/hello", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hi"))
	})
	req := httptest.NewRequestWithContext(b.Context(), http.MethodGet, "/hello", nil)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
}

// BenchmarkServeHTTPParallel measures concurrent ServeHTTP calls; each worker
// builds its own request/recorder pair.
func BenchmarkServeHTTPParallel(b *testing.B) {
	r := benchRouter(b)
	r.Handle("GET", "/hello/{name}", func(w http.ResponseWriter, req *http.Request) {
		_, _ = w.Write([]byte(router.Param(req, "name")))
	})
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/hello/world", nil)
			r.ServeHTTP(httptest.NewRecorder(), req)
		}
	})
}

// BenchmarkRegister measures the exported registry-wiring entrypoint.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		Register()
	}
}
