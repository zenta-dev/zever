package middleware

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/ratelimit"
)

// errHandlerRan marks a test handler that must never execute.
var errHandlerRan = errors.New("handler ran")

// fakeLimiter is a minimal ratelimit.Limiter fake: decision/err are fixed per
// test, and every Allow call is recorded.
type fakeLimiter struct {
	decision ratelimit.Decision
	err      error
	calls    []fakeAllowCall
}

type fakeAllowCall struct {
	key    string
	tokens float64
}

func (f *fakeLimiter) Allow(_ context.Context, key string, tokens float64) (ratelimit.Decision, error) {
	f.calls = append(f.calls, fakeAllowCall{key: key, tokens: tokens})
	return f.decision, f.err
}

func (f *fakeLimiter) Reset(context.Context, string) error { return nil }
func (f *fakeLimiter) Close() error                        { return nil }
func (f *fakeLimiter) Name() string                        { return "fake" }

var _ ratelimit.Limiter = (*fakeLimiter)(nil)

func allowDecision() ratelimit.Decision {
	return ratelimit.Decision{Allowed: true, Remaining: 9}
}

func denyDecision(retryAfter time.Duration) ratelimit.Decision {
	return ratelimit.Decision{Allowed: false, RetryAfter: retryAfter}
}

func TestRateLimit_allowsWhenLimiterAllows(t *testing.T) {
	limiter := &fakeLimiter{decision: allowDecision()}

	handler := RateLimit(limiter, RemoteAddrKey)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5555"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(limiter.calls) != 1 || limiter.calls[0].key != "1.2.3.4" {
		t.Fatalf("Allow calls = %v, want one call keyed by the client IP", limiter.calls)
	}
	if limiter.calls[0].tokens != 1 {
		t.Fatalf("Allow tokens = %v, want 1", limiter.calls[0].tokens)
	}
}

func TestRateLimit_deniesWith429RetryAfterAndBody(t *testing.T) {
	limiter := &fakeLimiter{decision: denyDecision(30 * time.Second)}

	handler := RateLimit(limiter, RemoteAddrKey)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler must not run when the limiter denies the request")
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5555"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("Retry-After = %q, want 30", got)
	}
	if got := rec.Body.String(); got != "{\"error\":\"rate limit exceeded\"}\n" {
		t.Fatalf("body = %q, want exact deny JSON", got)
	}
}

func TestRateLimit_denyRetryAfterCeilsSubSecond(t *testing.T) {
	// RFC 9110 Retry-After is delay-seconds: a sub-second window must ceil
	// to 1, never truncate to 0 (which would invite an immediate retry).
	limiter := &fakeLimiter{decision: denyDecision(500 * time.Millisecond)}

	handler := RateLimit(limiter, RemoteAddrKey)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler must not run when the limiter denies the request")
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5555"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1 (ceiled)", got)
	}
}

func TestRateLimit_failsOpenOnLimiterError(t *testing.T) {
	limiter := &fakeLimiter{err: errors.New("redis down")}

	ran := false
	handler := RateLimit(limiter, RemoteAddrKey)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5555"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !ran {
		t.Fatalf("a rate-limiter error must fail open, not block the request")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRateLimit_failOpenExplicitOnLimiterError(t *testing.T) {
	limiter := &fakeLimiter{err: errors.New("redis down")}

	ran := false
	handler := RateLimit(limiter, RemoteAddrKey, WithFailMode(FailOpen))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5555"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !ran {
		t.Fatalf("explicit FailOpen must pass the request through on a limiter error")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRateLimit_failClosedOnLimiterError(t *testing.T) {
	limiter := &fakeLimiter{err: errors.New("redis down")}

	handler := RateLimit(limiter, RemoteAddrKey, WithFailMode(FailClosed))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("handler must not run when FailClosed and the limiter errors")
	}))

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.RemoteAddr = "1.2.3.4:5555"

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Body.String(); got != "{\"error\":\"rate limit exceeded\"}\n" {
		t.Fatalf("body = %q, want exact deny JSON", got)
	}
}

func TestRemoteAddrKey(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"host port pair strips port", "1.2.3.4:5555", "1.2.3.4"},
		{"ipv6 pair strips port", "[::1]:80", "::1"},
		{"no port falls back to raw", "1.2.3.4", "1.2.3.4"},
		{"empty falls back to raw", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if got := RemoteAddrKey(r); got != tt.want {
				t.Fatalf("RemoteAddrKey(%q) = %q, want %q", tt.remoteAddr, got, tt.want)
			}
		})
	}
}

type rawAddr string

func (a rawAddr) Network() string { return "tcp" }
func (a rawAddr) String() string  { return string(a) }

func TestPeerAddrKey(t *testing.T) {
	tests := []struct {
		name string
		peer *peer.Peer
		want string
	}{
		{"tcp peer strips port", &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("10.0.0.5"), Port: 8080}}, "10.0.0.5"},
		{"addr without port falls back to raw", &peer.Peer{Addr: rawAddr("10.0.0.5")}, "10.0.0.5"},
		{"unparsable falls back to raw", &peer.Peer{Addr: rawAddr("weird::addr::")}, "weird::addr::"},
		{"absent peer is unknown", nil, "unknown"},
		{"nil addr is unknown", &peer.Peer{}, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.peer != nil {
				ctx = peer.NewContext(ctx, tt.peer)
			}
			if got := PeerAddrKey(ctx); got != tt.want {
				t.Fatalf("PeerAddrKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRateLimitUnaryServerInterceptor_deniesWithResourceExhausted(t *testing.T) {
	limiter := &fakeLimiter{decision: denyDecision(time.Second)}
	interceptor := RateLimitUnaryServerInterceptor(limiter, func(context.Context) string { return "key" })

	handler := func(context.Context, any) (any, error) {
		t.Error("handler must not run when the limiter denies the call")
		return nil, errHandlerRan
	}

	_, err := interceptor(t.Context(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("err = %v, want a gRPC status error", err)
	}
	if st.Code() != codes.ResourceExhausted {
		t.Fatalf("code = %v, want codes.ResourceExhausted", st.Code())
	}
	if st.Message() != "rate limit exceeded" {
		t.Fatalf("message = %q, want %q", st.Message(), "rate limit exceeded")
	}
	if len(limiter.calls) != 1 || limiter.calls[0].key != "key" {
		t.Fatalf("Allow calls = %v, want one call with the keyFunc key", limiter.calls)
	}
}

func TestRateLimitUnaryServerInterceptor_allowsThrough(t *testing.T) {
	limiter := &fakeLimiter{decision: allowDecision()}
	interceptor := RateLimitUnaryServerInterceptor(limiter, func(context.Context) string { return "key" })

	handler := func(_ context.Context, req any) (any, error) { return req, nil }

	resp, err := interceptor(t.Context(), "ok", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)
	if err != nil || resp != "ok" {
		t.Fatalf("resp, err = %v, %v, want %q, nil", resp, err, "ok")
	}
}

func TestRateLimitUnaryServerInterceptor_failsOpenOnLimiterError(t *testing.T) {
	limiter := &fakeLimiter{err: errors.New("redis down")}
	interceptor := RateLimitUnaryServerInterceptor(limiter, func(context.Context) string { return "key" })

	handler := func(_ context.Context, req any) (any, error) { return req, nil }

	resp, err := interceptor(t.Context(), "ok", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)
	if err != nil || resp != "ok" {
		t.Fatalf("resp, err = %v, %v, want %q, nil", resp, err, "ok")
	}
}

func TestRateLimitUnaryServerInterceptor_failOpenExplicitOnLimiterError(t *testing.T) {
	limiter := &fakeLimiter{err: errors.New("redis down")}
	interceptor := RateLimitUnaryServerInterceptor(limiter, func(context.Context) string { return "key" }, WithFailMode(FailOpen))

	handler := func(_ context.Context, req any) (any, error) { return req, nil }

	resp, err := interceptor(t.Context(), "ok", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)
	if err != nil || resp != "ok" {
		t.Fatalf("resp, err = %v, %v, want %q, nil", resp, err, "ok")
	}
}

func TestRateLimitUnaryServerInterceptor_failClosedOnLimiterError(t *testing.T) {
	limiter := &fakeLimiter{err: errors.New("redis down")}
	interceptor := RateLimitUnaryServerInterceptor(limiter, func(context.Context) string { return "key" }, WithFailMode(FailClosed))

	handler := func(context.Context, any) (any, error) {
		t.Error("handler must not run when FailClosed and the limiter errors")
		return nil, errHandlerRan
	}

	_, err := interceptor(t.Context(), "req", &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}, handler)

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("err = %v, want a gRPC status error", err)
	}
	if st.Code() != codes.ResourceExhausted {
		t.Fatalf("code = %v, want codes.ResourceExhausted", st.Code())
	}
}
