// Package middlewaretest provides the conformance kit for the middleware chaining contract.
//
// Middleware owns no adapter registry and no config section: it wraps
// already-resolved instances through plain constructors. The kit is
// contract-shaped rather than factory-shaped: Conformance takes no
// factory and exercises CORS, Recover, Timeout, RateLimit, and
// Tracing composition over in-kit stubs, so every present and future
// constructor keeps the same denial envelopes and chain order.
package middlewaretest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/log/noop"
	"github.com/zenta-dev/zever/core/middleware"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/ratelimit"
)

// DefaultTimeoutBounds bounds the Timeout probes: the fast handler
// must finish well inside it and the slow handler must trip it.
const DefaultTimeoutBounds = 2 * time.Second

// Conformance verifies the middleware chaining contract: CORS
// allow/preflight/deny shapes, Recover panic envelope, Timeout
// fast/slow shapes, RateLimit allow/deny/fail-open shapes, Tracing
// passthrough, and full-chain composition. Tests serve over
// httptest (no external network) and never call time.Sleep; the
// slow Timeout handler blocks on context cancellation, never a
// sleep.
func Conformance(t *testing.T) {
	t.Helper()

	t.Run("CORS", conformanceCORS)
	t.Run("Recover", conformanceRecover)
	t.Run("Timeout", conformanceTimeout)
	t.Run("RateLimit", conformanceRateLimit)
	t.Run("Tracing", conformanceTracing)
	t.Run("Chain", conformanceChain)
}

// okHandler answers 200 with body "ok".
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
}

// bodyOf drains rec into a string or fails the test.
func bodyOf(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()

	data, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}

	return string(data)
}

func conformanceCORS(t *testing.T) {
	t.Helper()

	mw := middleware.CORS(middleware.Options{
		AllowedOrigins: []string{"https://kit.example"},
		AllowedMethods: []string{"GET"},
	})

	// Simple allowed-origin request passes through with the allow header.
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil)
	req.Header.Set("Origin", "https://kit.example")
	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("CORS allow code = %d, want 200", rec.Code)
	}

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://kit.example" {
		t.Errorf("Access-Control-Allow-Origin = %q, want origin", got)
	}

	// Preflight on the allowed origin short-circuits with 204.
	pre := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/kit", nil)
	pre.Header.Set("Origin", "https://kit.example")
	pre.Header.Set("Access-Control-Request-Method", "GET")
	prec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(prec, pre)

	if prec.Code != http.StatusNoContent {
		t.Errorf("CORS preflight code = %d, want 204", prec.Code)
	}

	// Disallowed origin is never reflected: request passes through bare.
	foreign := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil)
	foreign.Header.Set("Origin", "https://evil.example")
	frec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(frec, foreign)

	if frec.Code != http.StatusOK {
		t.Errorf("CORS foreign code = %d, want 200 passthrough", frec.Code)
	}

	if got := frec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("CORS foreign allow-origin = %q, want empty", got)
	}

	// Invalid options fail closed with the fixed 500 envelope.
	broken := middleware.CORS(middleware.Options{})
	brec := httptest.NewRecorder()
	broken(okHandler()).ServeHTTP(brec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if brec.Code != http.StatusInternalServerError {
		t.Errorf("CORS invalid code = %d, want 500", brec.Code)
	}

	if got := bodyOf(t, brec); !strings.Contains(got, `"error"`) {
		t.Errorf("CORS invalid body = %q, want error envelope", got)
	}
}

func conformanceRecover(t *testing.T) {
	t.Helper()

	mw := middleware.Recover(noop.New())

	panicking := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("kit panic")
	})

	rec := httptest.NewRecorder()
	mw(panicking).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Recover code = %d, want 500", rec.Code)
	}

	got := bodyOf(t, rec)
	if !strings.Contains(got, `"error"`) {
		t.Errorf("Recover body = %q, want error envelope", got)
	}

	if strings.Contains(got, "kit panic") {
		t.Errorf("Recover body leaks panic value: %q", got)
	}

	// Non-panicking handlers pass through untouched.
	healthy := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(healthy, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if healthy.Code != http.StatusOK {
		t.Errorf("Recover passthrough code = %d, want 200", healthy.Code)
	}
}

func conformanceTimeout(t *testing.T) {
	t.Helper()

	mw := middleware.Timeout(DefaultTimeoutBounds)

	// Fast handler result passes through.
	fast := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(fast, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if fast.Code != http.StatusOK {
		t.Errorf("Timeout fast code = %d, want 200", fast.Code)
	}

	// Slow handler blocks on ctx done (no sleep) and trips 504.
	slow := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
	srec := httptest.NewRecorder()

	tight := middleware.Timeout(50 * time.Millisecond)
	tight(slow).ServeHTTP(srec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if srec.Code != http.StatusGatewayTimeout {
		t.Errorf("Timeout slow code = %d, want 504", srec.Code)
	}

	if got := bodyOf(t, srec); !strings.Contains(got, `"error"`) {
		t.Errorf("Timeout slow body = %q, want error envelope", got)
	}
}

func conformanceRateLimit(t *testing.T) {
	t.Helper()

	// Allow then deny: first request passes, second is rejected with
	// 429 plus a Retry-After hint.
	limiter := &stubLimiter{allowed: []bool{true, false}}
	mw := middleware.RateLimit(limiter, middleware.RemoteAddrKey)

	first := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(first, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if first.Code != http.StatusOK {
		t.Errorf("RateLimit allow code = %d, want 200", first.Code)
	}

	second := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(second, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("RateLimit deny code = %d, want 429", second.Code)
	}

	if second.Header().Get("Retry-After") == "" {
		t.Error("RateLimit deny missing Retry-After header")
	}

	if got := bodyOf(t, second); !strings.Contains(got, `"error"`) {
		t.Errorf("RateLimit deny body = %q, want error envelope", got)
	}

	// Limiter failure fails open by default.
	failing := middleware.RateLimit(&stubLimiter{err: errors.New("kit limiter down")}, middleware.RemoteAddrKey)
	frec := httptest.NewRecorder()
	failing(okHandler()).ServeHTTP(frec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if frec.Code != http.StatusOK {
		t.Errorf("RateLimit fail-open code = %d, want 200", frec.Code)
	}

	// FailClosed rejects on limiter error instead.
	closed := middleware.RateLimit(&stubLimiter{err: errors.New("kit limiter down")}, middleware.RemoteAddrKey, middleware.WithFailMode(middleware.FailClosed))
	crec := httptest.NewRecorder()
	closed(okHandler()).ServeHTTP(crec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if crec.Code != http.StatusTooManyRequests {
		t.Errorf("RateLimit fail-closed code = %d, want 429", crec.Code)
	}
}

func conformanceTracing(t *testing.T) {
	t.Helper()

	provider := &stubProvider{}
	mw := middleware.Tracing(provider)

	rec := httptest.NewRecorder()
	mw(okHandler()).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("Tracing code = %d, want 200", rec.Code)
	}

	if !provider.traced {
		t.Error("Tracing never started a span")
	}
}

func conformanceChain(t *testing.T) {
	t.Helper()

	// Documented chain order: CORS outermost, then Recover, then
	// Timeout, then RateLimit. A preflight short-circuits before the
	// inner layers; a panic inside still converts to 500.
	//
	// The panic leg omits Timeout on purpose: Timeout runs its inner
	// chain in a worker goroutine, and a panic there cannot be caught
	// by an outer Recover in the parent goroutine (same-goroutine
	// rule). Panic conversion with Timeout in path stays
	// adapter-owned; the kit pins Recover conversion without it.
	inner := middleware.RateLimit(&stubLimiter{allowed: []bool{true}}, middleware.RemoteAddrKey)
	timed := middleware.Timeout(DefaultTimeoutBounds)
	recovered := middleware.Recover(noop.New())
	cors := middleware.CORS(middleware.Options{
		AllowedOrigins: []string{"https://kit.example"},
		AllowedMethods: []string{"GET"},
	})

	chained := cors(recovered(timed(inner(okHandler()))))

	pre := httptest.NewRequestWithContext(t.Context(), http.MethodOptions, "/kit", nil)
	pre.Header.Set("Origin", "https://kit.example")
	pre.Header.Set("Access-Control-Request-Method", "GET")
	prec := httptest.NewRecorder()
	chained.ServeHTTP(prec, pre)

	if prec.Code != http.StatusNoContent {
		t.Errorf("chain preflight code = %d, want 204", prec.Code)
	}

	panicking := cors(recovered(inner(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("kit chain panic")
	}))))
	preq := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/kit", nil)
	preq.Header.Set("Origin", "https://kit.example")
	panrec := httptest.NewRecorder()
	panicking.ServeHTTP(panrec, preq)

	if panrec.Code != http.StatusInternalServerError {
		t.Errorf("chain panic code = %d, want 500", panrec.Code)
	}
}

// stubLimiter is a scripted ratelimit.Limiter: each Allow consumes
// the next entry of allowed, defaulting to allow once drained.
type stubLimiter struct {
	allowed []bool
	err     error
	calls   int
}

func (s *stubLimiter) Allow(_ context.Context, _ string, _ float64) (ratelimit.Decision, error) {
	if s.err != nil {
		return ratelimit.Decision{}, s.err
	}

	allowed := true
	if s.calls < len(s.allowed) {
		allowed = s.allowed[s.calls]
	}

	s.calls++

	if !allowed {
		return ratelimit.Decision{Allowed: false, RetryAfter: time.Second}, nil
	}

	return ratelimit.Decision{Allowed: true, Remaining: 1}, nil
}

// Reset clears the script cursor.
func (s *stubLimiter) Reset(_ context.Context, _ string) error { return nil }

// Close releases no resources.
func (s *stubLimiter) Close() error { return nil }

// Name identifies the stub limiter.
func (s *stubLimiter) Name() string { return "kit-stub" }

// stubProvider records whether Tracing started a span.
type stubProvider struct {
	traced bool
}

// Tracer returns a span-recording stub tracer.
func (p *stubProvider) Tracer(_ string) observability.Tracer { return &stubTracer{parent: p} }

// Meter returns a no-op stub meter.
func (p *stubProvider) Meter(_ string) observability.Metrics { return stubMetrics{} }

// Shutdown releases no resources.
func (p *stubProvider) Shutdown(_ context.Context) error { return nil }

// stubTracer marks its provider traced on Start.
type stubTracer struct {
	parent *stubProvider
}

// Start records the trace and returns a stub span.
func (s *stubTracer) Start(ctx context.Context, _ string) (context.Context, observability.Span) {
	s.parent.traced = true

	return ctx, stubSpan{}
}

// Shutdown releases no resources.
func (s *stubTracer) Shutdown(_ context.Context) error { return nil }

// stubSpan discards span data.
type stubSpan struct{}

// SetAttributes discards attributes.
func (stubSpan) SetAttributes(...observability.Attr) {}

// RecordError discards the error.
func (stubSpan) RecordError(error) {}

// End ends the stub span.
func (stubSpan) End() {}

// stubMetrics discards measurements.
type stubMetrics struct{}

// Counter discards the measurement.
func (stubMetrics) Counter(_ context.Context, _ string, _ float64, _ ...observability.Attr) error {
	return nil
}

// Gauge discards the measurement.
func (stubMetrics) Gauge(_ context.Context, _ string, _ float64, _ ...observability.Attr) error {
	return nil
}

// Histogram discards the measurement.
func (stubMetrics) Histogram(_ context.Context, _ string, _ float64, _ ...observability.Attr) error {
	return nil
}

// Shutdown releases no resources.
func (stubMetrics) Shutdown(_ context.Context) error { return nil }
