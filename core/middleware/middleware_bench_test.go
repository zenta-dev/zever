package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/log/noop"
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

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
