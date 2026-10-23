package middleware

import (
	"context"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/core/ratelimit"
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

// ParseTrustedProxies parses trusted-proxy CIDRs (or bare IPs) for
// ClientIPKey. It is ratelimit.ParseTrustedProxies under the middleware
// import path so generated servers need only the middleware package.
func ParseTrustedProxies(entries []string) ([]netip.Prefix, error) {
	prefixes, err := ratelimit.ParseTrustedProxies(entries)
	if err != nil {
		return nil, err
	}

	return prefixes, nil
}

// ClientIPKey returns an HTTP rate-limit key func that honors
// X-Forwarded-For only from trusted peers. When the peer address falls
// inside trusted, the key is the rightmost untrusted X-Forwarded-For
// entry (a spoofed left entry planted behind a trusted proxy cannot
// shadow the address the proxy appended); when every entry is trusted,
// the leftmost (original client) wins. A peer outside trusted, a missing
// or fully malformed header, or an empty trust list falls back to
// RemoteAddrKey, which is also what a nil trust list yields on every
// request.
func ClientIPKey(trusted []netip.Prefix) func(*http.Request) string {
	return func(r *http.Request) string {
		if !addrInPrefixes(remoteHost(r.RemoteAddr), trusted) {
			return RemoteAddrKey(r)
		}

		if client, ok := rightmostUntrusted(strings.Join(r.Header.Values("X-Forwarded-For"), ","), trusted); ok {
			return client
		}

		return RemoteAddrKey(r)
	}
}

// remoteHost strips the port from addr, falling back to addr itself.
func remoteHost(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}

	return host
}

// normalizeAddr parses s as an IP for prefix matching: brackets, port,
// and zone are stripped and IPv4-mapped IPv6 is unmapped so it matches
// IPv4 prefixes. ok is false for anything unparsable.
func normalizeAddr(s string) (addr netip.Addr, ok bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, false
	}

	if parsed, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
		return parsed.Unmap().WithZone(""), true
	}

	if host, _, err := net.SplitHostPort(s); err == nil {
		if parsed, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
			return parsed.Unmap().WithZone(""), true
		}
	}

	return netip.Addr{}, false
}

// addrInPrefixes reports whether host parses to an IP inside any prefix.
func addrInPrefixes(host string, trusted []netip.Prefix) bool {
	addr, ok := normalizeAddr(host)
	if !ok {
		return false
	}

	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
}

// rightmostUntrusted scans an X-Forwarded-For header right-to-left for the
// first entry outside trusted. Malformed entries are skipped. When every
// valid entry is trusted, the leftmost (original client) is returned.
// ok is false when no entry parses at all.
func rightmostUntrusted(header string, trusted []netip.Prefix) (client string, ok bool) {
	if strings.TrimSpace(header) == "" {
		return "", false
	}

	var first string

	// Manual scan to avoid strings.Split allocation.
	// Scan left-to-right, tracking the rightmost untrusted.
	start := 0
	for i := 0; i <= len(header); i++ {
		if i == len(header) || header[i] == ',' {
			entry := strings.TrimSpace(header[start:i])
			start = i + 1

			addr, valid := normalizeAddr(entry)
			if !valid {
				continue
			}

			if first == "" {
				first = addr.String()
			}

			if !prefixContains(trusted, addr) {
				client, ok = addr.String(), true
			}
		}
	}

	// Entries were scanned left-to-right, so client now holds the
	// rightmost untrusted one; fall back to the original client when all
	// entries are trusted.
	if !ok && first != "" {
		return first, true
	}

	return client, ok
}

// prefixContains reports whether addr falls inside any prefix.
func prefixContains(trusted []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range trusted {
		if prefix.Contains(addr) {
			return true
		}
	}

	return false
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
