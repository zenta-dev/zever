// Package httpclient provides a shared HTTP client constructor with a TLS 1.2
// floor on a cloned default transport, plus bounded body reads.
package httpclient

import (
	"context"
	"crypto/tls"
	"errors"
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

// ErrTooLarge is returned (via TooLargeError) when ReadLimited exceeds limit.
var ErrTooLarge = errors.New("httpclient: response body too large")

// ErrPrivateAddress is returned when SafeDialContext resolves a private
// address and allowPrivate is false.
var ErrPrivateAddress = errors.New("httpclient: refusing to dial private address")

// ErrNoAddresses is returned when SafeDialContext has no addresses to dial.
var ErrNoAddresses = errors.New("httpclient: no addresses to dial")

// TooLargeError reports a body exceeding a byte limit.
type TooLargeError struct {
	// Limit is the maximum allowed bytes.
	Limit int64
	// Size is the bytes observed (capped at Limit+1).
	Size int64
}

// Error returns a human-readable description of the limit breach.
func (e *TooLargeError) Error() string {
	return fmt.Sprintf("httpclient: response body too large: %d bytes exceeds limit %d", e.Size, e.Limit)
}

// Is reports whether target is ErrTooLarge.
func (e *TooLargeError) Is(target error) bool {
	return target == ErrTooLarge
}

// Unwrap exposes ErrTooLarge for errors.Is and errors.As.
func (e *TooLargeError) Unwrap() error {
	return ErrTooLarge
}

// config holds NewClient options.
type config struct {
	transport      http.RoundTripper
	useTransport   bool
	safeDial       bool
	safeDialSet    bool
	allowPrivate   bool
	noRedirect     bool
	insecureVerify bool
	insecureSet    bool
}

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
// allowPrivate is true. WithSafeDial has no effect with WithTransport.
func WithSafeDial(allowPrivate bool) Option {
	return func(c *config) {
		c.safeDial = true
		c.safeDialSet = true
		c.allowPrivate = allowPrivate
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
// config. WithInsecureSkipVerify is for loopback test servers only and has no
// effect with WithTransport.
func WithInsecureSkipVerify(skip bool) Option {
	return func(c *config) {
		c.insecureVerify = skip
		c.insecureSet = true
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
				clone.DialContext = SafeDialContext(cfg.allowPrivate)
			}
			clone.MaxIdleConnsPerHost = DefaultMaxIdleConnsPerHost
			return newHTTPClient(timeout, clone, cfg.noRedirect)
		}
		return newHTTPClient(timeout, cfg.transport, cfg.noRedirect)
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
		tr.DialContext = SafeDialContext(cfg.allowPrivate)
	}
	tr.MaxIdleConnsPerHost = DefaultMaxIdleConnsPerHost
	return newHTTPClient(timeout, tr, cfg.noRedirect)
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
func newHTTPClient(timeout time.Duration, rt http.RoundTripper, noRedirect bool) *http.Client {
	c := &http.Client{Timeout: timeout, Transport: traceRoundTripper{base: rt}}
	if noRedirect {
		c.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	return c
}

// NewSafeClient returns an *http.Client enforcing a TLS 1.2 minimum, refusing
// private-address dials unless allowPrivate is true, never following
// redirects, and applying timeout to the whole request.
func NewSafeClient(timeout time.Duration, allowPrivate bool) *http.Client {
	return NewClient(timeout, WithSafeDial(allowPrivate), WithNoRedirect())
}

// SafeDialContext returns a DialContext function refusing private addresses
// unless allowPrivate is true. SafeDialContext resolves names, checks every
// returned address, then dials a validated IP directly so a second lookup
// cannot return a different (private) address between check and dial.
func SafeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
			port = ""
		}
		var ips []net.IP
		if ip := net.ParseIP(host); ip != nil {
			ips = []net.IP{ip}
		} else {
			addrs, lookupErr := net.DefaultResolver.LookupIPAddr(ctx, host)
			if lookupErr != nil {
				return nil, fmt.Errorf("httpclient: host lookup failed for %q: %w", host, lookupErr)
			}
			for _, a := range addrs {
				ips = append(ips, a.IP)
			}
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
		var dialer net.Dialer
		var lastErr error
		for _, ip := range ips {
			target := addr
			if port != "" {
				target = net.JoinHostPort(ip.String(), port)
			}
			conn, dialErr := dialer.DialContext(ctx, network, target)
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

// IsPrivateIP reports whether ip is loopback, unspecified, link-local, or
// private (10/8, 172.16/12, 192.168/16, fc00::/7). IsPrivateIP returns false
// for nil.
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 10:
			return true
		case ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31:
			return true
		case ip4[0] == 192 && ip4[1] == 168:
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
