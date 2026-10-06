package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// BenchmarkCacheRoundTrip measures the cache set/get demo hot path.
func BenchmarkCacheRoundTrip(b *testing.B) {
	h := benchSetup(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		setReq := httptest.NewRequestWithContext(ctx, http.MethodPost, "/demo/cache/bench-key", strings.NewReader("bench-value"))
		setRec := httptest.NewRecorder()
		h.ServeHTTP(setRec, setReq)
		if setRec.Code != http.StatusOK {
			b.Fatalf("set: %d", setRec.Code)
		}
		getReq := httptest.NewRequestWithContext(ctx, http.MethodGet, "/demo/cache/bench-key", nil)
		getRec := httptest.NewRecorder()
		h.ServeHTTP(getRec, getReq)
		if getRec.Code != http.StatusOK {
			b.Fatalf("get: %d", getRec.Code)
		}
	}
}

// BenchmarkSessionRoundTrip measures the session create/get demo hot path.
func BenchmarkSessionRoundTrip(b *testing.B) {
	h := benchSetup(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		createReq := httptest.NewRequestWithContext(ctx, http.MethodPost, "/demo/session", strings.NewReader(`{"user":"bench"}`))
		createReq.Header.Set("Content-Type", "application/json")
		createRec := httptest.NewRecorder()
		h.ServeHTTP(createRec, createReq)
		if createRec.Code != http.StatusCreated {
			b.Fatalf("create: %d", createRec.Code)
		}
	}
}
