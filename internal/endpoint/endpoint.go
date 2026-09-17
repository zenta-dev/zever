// Package endpoint provides shared validation for outbound HTTP(S)
// endpoint URLs: scheme allow-listing, host presence, loopback, userinfo,
// whitespace, and query/fragment strictness.
//
// ValidateURL parses raw, enforces the policy selected by opts, and returns
// the normalized (url.URL.String) form on success. Failures are reported
// with sentinel errors that all wrap ErrInvalid, so callers can map them to
// domain error types with errors.Is while preserving their own messages.
package endpoint

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ErrInvalid is wrapped by every validation failure from ValidateURL.
var ErrInvalid = errors.New("endpoint: invalid URL")

var (
	// ErrEmpty reports an empty URL string.
	ErrEmpty = fmt.Errorf("%w: URL is empty", ErrInvalid)
	// ErrWhitespace reports rejected whitespace in the raw URL.
	ErrWhitespace = fmt.Errorf("%w: URL must not contain whitespace", ErrInvalid)
	// ErrParse reports a URL that net/url cannot parse.
	ErrParse = fmt.Errorf("%w: URL cannot be parsed", ErrInvalid)
	// ErrNoScheme reports a URL with no scheme.
	ErrNoScheme = fmt.Errorf("%w: URL must include scheme", ErrInvalid)
	// ErrNoHost reports a URL with no host.
	ErrNoHost = fmt.Errorf("%w: URL must include host", ErrInvalid)
	// ErrUnsupportedScheme reports a scheme other than http or https.
	ErrUnsupportedScheme = fmt.Errorf("%w: URL scheme must be http or https", ErrInvalid)
	// ErrInsecureScheme reports plain http when only https (or loopback
	// http) is permitted.
	ErrInsecureScheme = fmt.Errorf("%w: URL must use https scheme", ErrInvalid)
	// ErrUserinfo reports a URL carrying user info when rejected.
	ErrUserinfo = fmt.Errorf("%w: URL must not contain user info", ErrInvalid)
	// ErrQueryFragment reports a URL carrying a query string or fragment
	// when rejected.
	ErrQueryFragment = fmt.Errorf("%w: URL must not contain a query string or fragment", ErrInvalid)
)

// config holds ValidateURL options.
type config struct {
	allowInsecure       bool
	allowLoopbackHTTP   bool
	allowAnyScheme      bool
	rejectUserinfo      bool
	rejectWhitespace    bool
	rejectQueryFragment bool
}

// Option configures ValidateURL.
type Option func(*config)

// WithAllowInsecure permits plain http in addition to https. Pass the
// caller's AllowInsecure flag through to gate http on explicit opt-in.
func WithAllowInsecure(allow bool) Option {
	return func(c *config) {
		c.allowInsecure = allow
	}
}

// WithAllowLoopbackHTTP permits plain http only when the host is loopback
// (localhost or a loopback IP). Non-loopback http still fails with
// ErrInsecureScheme.
func WithAllowLoopbackHTTP() Option {
	return func(c *config) {
		c.allowLoopbackHTTP = true
	}
}

// WithAllowAnyScheme accepts any non-empty scheme as long as a host is
// present. Without it, only https is accepted unless another option admits
// http.
func WithAllowAnyScheme() Option {
	return func(c *config) {
		c.allowAnyScheme = true
	}
}

// WithRejectUserinfo rejects URLs carrying user info.
func WithRejectUserinfo() Option {
	return func(c *config) {
		c.rejectUserinfo = true
	}
}

// WithRejectWhitespace rejects raw URLs containing spaces or control
// whitespace before parsing.
func WithRejectWhitespace() Option {
	return func(c *config) {
		c.rejectWhitespace = true
	}
}

// WithRejectQueryFragment rejects URLs carrying a query string or fragment.
func WithRejectQueryFragment() Option {
	return func(c *config) {
		c.rejectQueryFragment = true
	}
}

// ValidateURL parses raw as a URL, enforces scheme and host presence plus the
// policy selected by opts, and returns the normalized URL string. The
// returned error wraps ErrInvalid and one of the Err* sentinels above.
func ValidateURL(raw string, opts ...Option) (string, error) {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	if raw == "" {
		return "", ErrEmpty
	}

	if cfg.rejectWhitespace && strings.ContainsAny(raw, " \t\n\v\r") {
		return "", ErrWhitespace
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", ErrParse
	}

	if u.Scheme == "" {
		return "", ErrNoScheme
	}

	if u.Host == "" {
		return "", ErrNoHost
	}

	if !cfg.allowAnyScheme {
		switch u.Scheme {
		case "https":
		case "http":
			switch {
			case cfg.allowInsecure:
			case cfg.allowLoopbackHTTP && IsLoopbackHost(u.Hostname()):
			default:
				return "", ErrInsecureScheme
			}
		default:
			return "", ErrUnsupportedScheme
		}
	}

	if cfg.rejectUserinfo && u.User != nil {
		return "", ErrUserinfo
	}

	if cfg.rejectQueryFragment && (u.RawQuery != "" || u.Fragment != "") {
		return "", ErrQueryFragment
	}

	return u.String(), nil
}

// IsLoopbackHost reports whether host is "localhost" or a loopback IP such
// as 127.0.0.1 or ::1. Pass a bare hostname (no port or brackets); use
// (*url.URL).Hostname to derive one. Exact match only: a substring check
// would wrongly trust hosts like 127.0.0.1.evil.com.
func IsLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// IsLoopbackURL reports whether raw parses to a loopback host. It returns
// false for unparseable input.
func IsLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return IsLoopbackHost(u.Hostname())
}
