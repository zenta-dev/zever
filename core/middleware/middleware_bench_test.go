package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/log/noop"
)

// BenchmarkRequestLoggerAndTracingChained measures a request through
// RequestLogger(Tracing(handler)) — the chain order generated server wiring
// uses — to quantify the per-request overhead after newStatusRecorder was
// made idempotent (chained status-reading middleware reuses one recorder per
// request instead of double-wrapping).
func BenchmarkRequestLoggerAndTracingChained(b *testing.B) {
	logger := noop.New()
	provider, _, _ := newTraceTestProvider()

	handler := RequestLogger(logger)(Tracing(provider)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/widgets", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkCORSAllowedOrigin(b *testing.B) {
	handler := CORS(testCORSOptions())(okHandler())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/widgets", nil)
	req.Header.Set("Origin", "https://example.com")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkCORSPreflight(b *testing.B) {
	handler := CORS(testCORSOptions())(okHandler())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/widgets", nil)
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkRecoverPassthrough(b *testing.B) {
	handler := Recover(noop.New())(okHandler())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/widgets", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkTimeoutFastHandler(b *testing.B) {
	handler := Timeout(time.Minute)(okHandler())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/widgets", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkRateLimitAllow(b *testing.B) {
	handler := RateLimit(&fakeLimiter{decision: allowDecision()}, RemoteAddrKey)(okHandler())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/widgets", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

func BenchmarkTracing(b *testing.B) {
	provider, _, _ := newTraceTestProvider()

	handler := Tracing(provider)(okHandler())

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/widgets", nil)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
