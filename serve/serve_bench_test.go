package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/middleware"
	"github.com/zenta-dev/zever/core/router"
)

// BenchmarkMergeGRPCPolicies measures the policy folding hot path used
// during server boot when GRPCModule implementations declare policies.
func BenchmarkMergeGRPCPolicies(b *testing.B) {
	mods := make([]Module, 10)
	for i := range mods {
		mods[i] = &mockGRPCModule{
			name: "module" + string(rune('0'+i)),
			policies: map[string]authz.Policy{
				"/pkg.Service/Method" + string(rune('0'+i)): {},
			},
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = mergeGRPCPolicies(mods)
	}
}

// BenchmarkMaxBodyHandler measures the request-body bounding middleware
// hot path on the HTTP server.
func BenchmarkMaxBodyHandler(b *testing.B) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := maxBodyHandler(next)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		h.ServeHTTP(rec, req)
	}
}

// BenchmarkMiddlewareFold measures the middleware chain construction hot
// path used in runServe: Recover, RequestLogger, Tracing, SecurityHeaders,
// and optional RateLimit.
func BenchmarkMiddlewareFold(b *testing.B) {
	r := newStubRouter()
	logger := &benchmarkLogger{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r.Use(
			middleware.Recover(logger),
			middleware.RequestLogger(logger),
			middleware.Tracing(nil),
			middleware.SecurityHeaders(middleware.SecurityHeadersOptions{}),
		)
	}
}

// benchmarkLogger is a minimal log.Logger implementation for benchmarking.
type benchmarkLogger struct{}

func (b *benchmarkLogger) Debug() log.Event                         { return nil }
func (b *benchmarkLogger) Info() log.Event                          { return nil }
func (b *benchmarkLogger) Warn() log.Event                          { return nil }
func (b *benchmarkLogger) Error() log.Event                         { return nil }
func (b *benchmarkLogger) Fatal() log.Event                         { return nil }
func (b *benchmarkLogger) With() log.Context                        { return nil }
func (b *benchmarkLogger) WithContext(ctx context.Context) log.Logger { return b }
func (b *benchmarkLogger) Enabled(log.Level) bool                   { return false }
func (b *benchmarkLogger) Sync() error                              { return nil }
func (b *benchmarkLogger) Name() string                             { return "bench" }

// mockGRPCModule implements GRPCModule for benchmarking.
type mockGRPCModule struct {
	name     string
	policies map[string]authz.Policy
}

func (m *mockGRPCModule) Name() string       { return m.name }
func (m *mockGRPCModule) Mount(r router.Router, c *container.Container) error { return nil }
func (m *mockGRPCModule) GRPCPolicies() map[string]authz.Policy { return m.policies }