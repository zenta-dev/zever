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
)

// ErrTooLarge is returned (via TooLargeError) when ReadLimited exceeds limit.
var ErrTooLarge = errors.New("httpclient: response body too large")

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
			c := &http.Client{Timeout: timeout, Transport: clone}
			if cfg.noRedirect {
				c.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
					return http.ErrUseLastResponse
				}
			}
			return c
		}
		c := &http.Client{Timeout: timeout, Transport: cfg.transport}
		if cfg.noRedirect {
			c.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			}
		}
		return c
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
	c := &http.Client{Timeout: timeout, Transport: tr}
	if cfg.noRedirect {
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
// unless allowPrivate is true. SafeDialContext resolves names and checks every
// returned address, so a single private record blocks the dial.
func SafeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		var ips []net.IP
		if ip := net.ParseIP(host); ip != nil {
			ips = []net.IP{ip}
		} else {
			addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("httpclient: host lookup failed for %q: %w", host, err)
			}
			for _, a := range addrs {
				ips = append(ips, a.IP)
			}
		}
		if !allowPrivate {
			for _, ip := range ips {
				if IsPrivateIP(ip) {
					return nil, fmt.Errorf("httpclient: refusing to dial private address %q", host)
				}
			}
		}
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, addr)
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
func ReadLimited(body io.Reader, limit int64) ([]byte, error) {
	if limit < 0 {
		limit = 0
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, &TooLargeError{Limit: limit, Size: int64(len(data))}
	}
	return data, nil
}
