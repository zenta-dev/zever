// Package httpclient provides a shared HTTP client constructor with a TLS 1.2
// floor on a cloned default transport, plus bounded body reads.
package httpclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/zenta-dev/zever/shared/traceprop"
)

// DefaultMaxIdleConnsPerHost is the default maximum idle connections per host
// applied to cloned transports created by NewClient.
const DefaultMaxIdleConnsPerHost = 32

// maxReadChunk bounds each ReadLimited read call; the context is re-checked
// between chunks so cancellation aborts promptly.
const maxReadChunk = 32 << 10

// ErrTooLarge, ErrPrivateAddress, ErrNoAddresses, and TooLargeError live in
// errors.go.

// config holds NewClient options.
type config struct {
	transport      http.RoundTripper
	useTransport   bool
	safeDial       bool
	safeDialSet    bool
	allowPrivate   bool
	lookup         LookupFunc
	noRedirect     bool
	insecureVerify bool
	insecureSet    bool
	tracing        bool
}

// LookupFunc resolves host to addresses. LookupFunc is a seam for pinning
// DNS in tests (stub resolver) or split-horizon environments; a nil
// LookupFunc means the system resolver.
type LookupFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

// Option configures NewClient.
type Option func(*config)

// WithTransport overrides the RoundTripper. NewClient clones *http.Transport
// values and applies the TLS floor; other types are used directly (intended
// for tests with fakes).
func WithTransport(rt http.RoundTripper) Option {
	return func(c *config) {
		c.transport = rt
		c.useTransport = true
	}
}

// WithSafeDial installs a DialContext refusing private addresses unless
// allowPrivate is true. With a non-*http.Transport value WithSafeDial has
// no effect (the transport is used directly, see WithTransport).
func WithSafeDial(allowPrivate bool) Option {
	return func(c *config) {
		c.safeDial = true
		c.safeDialSet = true
		c.allowPrivate = allowPrivate
	}
}

// WithLookup sets the resolver the safe dial guard uses. A nil LookupFunc
// means the system resolver. WithLookup only takes effect with WithSafeDial
// on a cloned *http.Transport; it has no effect with a non-*http.Transport
// value (see WithTransport).
func WithLookup(lookup LookupFunc) Option {
	return func(c *config) {
		c.lookup = lookup
	}
}

// WithNoRedirect configures the client to never follow redirects, returning
// the 3xx response to the caller.
func WithNoRedirect() Option {
	return func(c *config) {
		c.noRedirect = true
	}
}

// WithInsecureSkipVerify sets InsecureSkipVerify on the cloned transport TLS
// config. WithInsecureSkipVerify is an explicit opt-in for self-signed test
// servers only; it has no effect with a non-*http.Transport value (see
// WithTransport). Callers must gate it behind their own insecure_skip_verify
// option and log a warning when enabled.
func WithInsecureSkipVerify(skip bool) Option {
	return func(c *config) {
		c.insecureVerify = skip
		c.insecureSet = true
	}
}

// WithTracing enables W3C trace-context injection on outgoing requests. It is
// opt-in so the default client keeps a plain *http.Transport (callers and
// tests assert concrete transport types); pass it for service-to-service
// calls that should join the caller's trace.
func WithTracing() Option {
	return func(c *config) {
		c.tracing = true
	}
}

// NewClient returns an *http.Client with timeout, a TLS 1.2 floor on a clone
// of http.DefaultTransport, and optional dial-guard and redirect policies.
func NewClient(timeout time.Duration, opts ...Option) *http.Client {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.useTransport {
		if tr, ok := cfg.transport.(*http.Transport); ok {
			clone := tr.Clone()
			if clone.TLSClientConfig == nil {
				clone.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
			} else if clone.TLSClientConfig.MinVersion < tls.VersionTLS12 {
				clone.TLSClientConfig.MinVersion = tls.VersionTLS12
			}
			if cfg.insecureSet {
				clone.TLSClientConfig.InsecureSkipVerify = cfg.insecureVerify
			}
			if cfg.safeDialSet {
				clone.DialContext = SafeDialContextWithLookup(cfg.allowPrivate, cfg.lookup)
			}
			clone.MaxIdleConnsPerHost = DefaultMaxIdleConnsPerHost
			return newHTTPClient(timeout, clone, cfg.noRedirect, cfg.tracing)
		}
		return newHTTPClient(timeout, cfg.transport, cfg.noRedirect, cfg.tracing)
	}
	var tr *http.Transport
	if dt, ok := http.DefaultTransport.(*http.Transport); ok && dt != nil {
		// Clone before inspecting: reading the shared global directly races
		// with another goroutine's Clone/use, whose once-guarded lazy init
		// writes to it. The clone is ours alone, so reads/writes below are
		// race-free.
		tr = dt.Clone()
	} else {
		tr = &http.Transport{}
	}
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else if tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		tr.TLSClientConfig.MinVersion = tls.VersionTLS12
	}
	if cfg.insecureSet {
		tr.TLSClientConfig.InsecureSkipVerify = cfg.insecureVerify
	}
	if cfg.safeDialSet {
		tr.DialContext = SafeDialContextWithLookup(cfg.allowPrivate, cfg.lookup)
	}
	tr.MaxIdleConnsPerHost = DefaultMaxIdleConnsPerHost
	return newHTTPClient(timeout, tr, cfg.noRedirect, cfg.tracing)
}

// traceRoundTripper injects W3C trace context from the request context onto
// outgoing requests. It is a no-op when the context carries no valid span, so
// it is always safe to install.
type traceRoundTripper struct {
	base http.RoundTripper
}

// RoundTrip injects trace headers and delegates to the wrapped transport.
func (t traceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if injected := Inject(req.Context(), req); injected != req {
		req = injected
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	return base.RoundTrip(req)
}

// Inject returns a copy of req carrying the W3C trace context from ctx in its
// headers. The original request is never mutated; when ctx holds no valid span
// the original request is returned unchanged.
func Inject(ctx context.Context, req *http.Request) *http.Request {
	hdrs := traceprop.Inject(ctx, nil)
	if len(hdrs) == 0 {
		return req
	}

	clone := req.Clone(ctx)
	for k, v := range hdrs {
		clone.Header.Set(k, v)
	}

	return clone
}

// newHTTPClient builds the client with the trace-injecting transport and the
// optional no-redirect policy, shared by every NewClient branch.
func newHTTPClient(timeout time.Duration, rt http.RoundTripper, noRedirect, tracing bool) *http.Client {
	transport := rt
	if tracing {
		transport = traceRoundTripper{base: rt}
	}

	c := &http.Client{Timeout: timeout, Transport: transport}
	if noRedirect {
		c.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return c
}

// NewSafeClient returns an *http.Client enforcing a TLS 1.2 minimum, refusing
// private-address dials unless allowPrivate is true, never following
// redirects, and applying timeout to the whole request. Trace injection is off
// by default; pass WithTracing to NewClient when you also want propagation.
func NewSafeClient(timeout time.Duration, allowPrivate bool, opts ...Option) *http.Client {
	base := make([]Option, 0, 2+len(opts))
	base = append(base, WithSafeDial(allowPrivate), WithNoRedirect())

	return NewClient(timeout, append(base, opts...)...)
}

// SafeDialContext returns a DialContext function refusing private addresses
// unless allowPrivate is true. SafeDialContext resolves names with the
// system resolver, checks every returned address, then dials a validated IP
// directly so a second lookup cannot return a different (private) address
// between check and dial.
func SafeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return SafeDialContextWithLookup(allowPrivate, nil)
}

// SafeDialContextWithLookup is SafeDialContext with an injectable resolver:
// a nil lookup means the system resolver, a non-nil one (stub resolvers in
// tests, pinned resolvers in split-horizon environments) replaces it.
func SafeDialContextWithLookup(allowPrivate bool, lookup LookupFunc) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return safeDialContext(allowPrivate, lookup, nil)
}

// safeDialContext builds the guard. A nil lookup means the system resolver;
// a nil dial means a fresh net.Dialer per address (tests inject a stub to
// assert the dial target without touching the network).
func safeDialContext(allowPrivate bool, lookup LookupFunc, dial func(ctx context.Context, network, addr string) (net.Conn, error)) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
			port = ""
		}
		ips, err := resolveIPs(ctx, host, lookup)
		if err != nil {
			return nil, err
		}
		if !allowPrivate {
			for _, ip := range ips {
				if IsPrivateIP(ip) {
					return nil, fmt.Errorf("httpclient: refusing to dial private address %q: %w", host, ErrPrivateAddress)
				}
			}
		}
		// Dial a validated IP rather than the original hostname: re-resolving
		// here would allow a DNS-rebinding TOCTOU between the check and dial.
		dialAddr := func(ip net.IP) string {
			if port != "" {
				return net.JoinHostPort(ip.String(), port)
			}
			return ip.String()
		}
		var lastErr error
		for _, ip := range ips {
			target := dialAddr(ip)
			var conn net.Conn
			var dialErr error
			if dial != nil {
				conn, dialErr = dial(ctx, network, target)
			} else {
				var d net.Dialer
				conn, dialErr = d.DialContext(ctx, network, target)
			}
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("httpclient: no addresses to dial for %q: %w", host, ErrNoAddresses)
		}
		return nil, lastErr
	}
}

// resolveIPs returns the dial candidates for host: the literal when host
// parses as an IP, otherwise a single resolver round-trip. The caller dials
// from the returned slice without resolving again.
func resolveIPs(ctx context.Context, host string, lookup LookupFunc) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}, nil
	}
	if lookup == nil {
		lookup = net.DefaultResolver.LookupIPAddr
	}
	addrs, lookupErr := lookup(ctx, host)
	if lookupErr != nil {
		return nil, fmt.Errorf("httpclient: host lookup failed for %q: %w", host, lookupErr)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	return ips, nil
}

// IsPrivateIP reports whether ip must never be dialed from an outbound
// adapter without an explicit allow_private opt-in: loopback, unspecified,
// link-local (including the 169.254.169.254 cloud metadata address),
// multicast, carrier-grade NAT (100.64.0.0/10), IANA special-purpose
// (192.0.0.0/24), benchmarking (198.18.0.0/15), current-network (0.0.0.0/8),
// reserved (240.0.0.0/4), RFC 1918 (10/8, 172.16/12, 192.168/16), and IPv6
// unique-local (fc00::/7). Documentation ranges (192.0.2.0/24,
// 198.51.100.0/24, 203.0.113.0/24, 2001:db8::/32) stay dialable: they are
// unroutable so they cannot reach internal services, and the ecosystem
// uses them as public examples. IsPrivateIP returns false for nil and for
// globally routable addresses. IPv4-mapped IPv6 addresses classify by
// their embedded IPv4 address.
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 0:
			return true
		case ip4[0] == 10:
			return true
		case ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127:
			return true
		case ip4[0] == 127:
			return true
		case ip4[0] == 169 && ip4[1] == 254:
			return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return true
		case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0:
			return true
		case ip4[0] == 192 && ip4[1] == 168:
			return true
		case ip4[0] == 198 && ip4[1] >= 18 && ip4[1] <= 19:
			return true
		case ip4[0] >= 224:
			return true
		}
		return false
	}
	if len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc {
		return true
	}
	return false
}

// ReadLimited reads body up to limit bytes. ReadLimited returns the bytes when
// within limit, or a *TooLargeError (matching ErrTooLarge) when exceeded.
// The context bounds the read: a canceled context aborts with its error.
func ReadLimited(ctx context.Context, body io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		limit = 0
	}
	lr := io.LimitReader(body, limit+1)

	// Read directly into the result buffer's spare capacity: no separate
	// scratch buffer is ever allocated, and bodies up to the initial capacity
	// cost exactly one allocation.
	bufCap := int64(maxReadChunk)
	if limit < int64(maxReadChunk) {
		bufCap = limit + 1
	}
	buf := make([]byte, 0, bufCap)

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if len(buf) == cap(buf) {
			grow := limit + 1
			if c := int64(cap(buf)); c <= limit/2 {
				grow = c * 2
			}

			nb := make([]byte, len(buf), grow)
			copy(nb, buf)
			buf = nb
		}

		n, rerr := lr.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]

		if int64(len(buf)) > limit {
			return nil, &TooLargeError{Limit: limit, Size: int64(len(buf))}
		}
		if rerr == io.EOF {
			return buf, nil
		}
		if rerr != nil {
			return nil, rerr
		}
	}
}
