package remote

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/i18n"
)

// benchKeySeq keeps fetch-benchmark keys unique so each iteration misses the
// cache and exercises the singleflight fetch path.
var benchKeySeq atomic.Int64

// benchAdapter builds an adapter against an in-process stub endpoint.
func benchAdapter(b *testing.B) *adapter {
	b.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/translate", func(w http.ResponseWriter, r *http.Request) {
		var req translateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(translateResponse{
			Translations: map[string]string{req.Key: "Bonjour, Ada!"},
		})
	})

	srv := httptest.NewServer(mux)
	b.Cleanup(srv.Close)

	ii, err := New(i18n.Options{Remote: i18n.RemoteOptions{Endpoint: srv.URL, AllowInsecure: true}})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	a, ok := ii.(*adapter)
	if !ok {
		b.Fatalf("New returned %T, want *adapter", ii)
	}

	b.Cleanup(func() { _ = a.Close() })

	return a
}

// BenchmarkTranslateCacheHit measures the cached fast path (no I/O).
func BenchmarkTranslateCacheHit(b *testing.B) {
	a := benchAdapter(b)
	ctx := b.Context()
	args := map[string]string{"name": "Ada"}

	if _, err := a.Translate(ctx, "fr", "hello", args); err != nil {
		b.Fatalf("warm Translate: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := a.Translate(ctx, "fr", "hello", args); err != nil {
			b.Fatalf("Translate: %v", err)
		}
	}
}

// BenchmarkTranslateFetch measures the HTTP fetch, decode, and cache store.
func BenchmarkTranslateFetch(b *testing.B) {
	a := benchAdapter(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("k%d", benchKeySeq.Add(1))

		if _, err := a.Translate(ctx, "fr", key, nil); err != nil {
			b.Fatalf("Translate: %v", err)
		}
	}
}
