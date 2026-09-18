package middleware

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/ratelimit"
)

// RemoteAddrKey is the default HTTP rate-limit key: the request's remote IP
// (host part only, port stripped), falling back to the raw RemoteAddr when it
// isn't a host:port pair. Split once per request; no allocs beyond that.
func RemoteAddrKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}

// PeerAddrKey is RemoteAddrKey's gRPC counterpart, deriving the caller's IP
// from the connection peer stored in ctx by grpc-go. Absent peer info (or a
// nil address) keys as "unknown"; an address without a port falls back to its
// raw string form.
func PeerAddrKey(ctx context.Context) string {
	p, ok := peer.FromContext(ctx)
	if !ok || p.Addr == nil {
		return "unknown"
	}

	host, _, err := net.SplitHostPort(p.Addr.String())
	if err != nil {
		return p.Addr.String()
	}

	return host
}

// FailMode controls what RateLimit and RateLimitUnaryServerInterceptor do
// when the underlying ratelimit.Limiter itself returns an error (as opposed
// to a normal allow/deny decision).
type FailMode int

const (
	// FailOpen passes the request through when the limiter errors. It is the
	// zero value so existing callers keep their current behavior: a broken
	// limiter must never itself take the API down.
	FailOpen FailMode = iota

	// FailClosed rejects the request when the limiter errors, for endpoints
	// where an unenforced limit is more dangerous than reduced availability.
	FailClosed
)

// options holds the configuration shared by RateLimit and
// RateLimitUnaryServerInterceptor.
type options struct {
	failMode FailMode
}

// Option configures RateLimit or RateLimitUnaryServerInterceptor.
type Option func(*options)

// WithFailMode sets the behavior used when the limiter itself errors.
// The default, FailOpen, is unchanged from before this option existed.
func WithFailMode(mode FailMode) Option {
	return func(o *options) { o.failMode = mode }
}

func buildOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	return o
}

// RateLimit returns HTTP middleware that calls limiter.Allow(ctx, keyFunc(r),
// 1) per request, responding 429 with a Retry-After header when denied. By
// default, a limiter failure fails OPEN (the request passes through): a
// broken limiter must never itself take the API down. Pass
// WithFailMode(FailClosed) to instead respond 429 on a limiter error, for
// high-sensitivity endpoints where an unenforced limit is worse than reduced
// availability.
func RateLimit(limiter ratelimit.Limiter, keyFunc func(*http.Request) string, opts ...Option) func(http.Handler) http.Handler {
	o := buildOptions(opts)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decision, err := limiter.Allow(r.Context(), keyFunc(r), 1)
			if err != nil {
				if o.failMode == FailClosed {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusTooManyRequests)
					if data, encErr := errorBodyCodec.Encode(errorBody{Error: "rate limit exceeded"}); encErr == nil {
						_, _ = w.Write(append(data, '\n'))
					}

					return
				}

				// Fail open, matching RateLimitUnaryServerInterceptor.
				next.ServeHTTP(w, r)

				return
			}

			if !decision.Allowed {
				// RFC 9110 Retry-After is delay-seconds: ceil so a
				// sub-second window becomes 1, never 0.
				w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(decision.RetryAfter.Seconds()))))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				if data, encErr := errorBodyCodec.Encode(errorBody{Error: "rate limit exceeded"}); encErr == nil {
					_, _ = w.Write(append(data, '\n'))
				}

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitUnaryServerInterceptor is RateLimit's gRPC counterpart, returning
// codes.ResourceExhausted when the limiter denies the call. By default, a
// limiter failure fails open (delegating to the handler): a broken limiter
// must never itself take the API down. Pass WithFailMode(FailClosed) to
// instead return codes.ResourceExhausted on a limiter error, for
// high-sensitivity endpoints where an unenforced limit is worse than reduced
// availability.
func RateLimitUnaryServerInterceptor(limiter ratelimit.Limiter, keyFunc func(context.Context) string, opts ...Option) grpc.UnaryServerInterceptor {
	o := buildOptions(opts)

	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		decision, err := limiter.Allow(ctx, keyFunc(ctx), 1)
		if err != nil {
			if o.failMode == FailClosed {
				return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
			}

			// Fail open, matching RateLimit's HTTP behavior.
			return handler(ctx, req)
		}

		if !decision.Allowed {
			return nil, status.Error(codes.ResourceExhausted, "rate limit exceeded")
		}

		return handler(ctx, req)
	}
}
