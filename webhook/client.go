package webhook

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"
)

// SafeDialContext returns a DialContext function for http.Transport that
// refuses to connect to private addresses unless allowPrivate is true.
// The host is taken from addr via net.SplitHostPort, falling back to the full
// addr when it carries no port. IP literals take the ParseIP fast path; names
// resolve via LookupIPAddr and every returned address is checked, so a single
// private record blocks the dial and closes the DNS-rebinding gap at connect
// time.
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
				return nil, fmt.Errorf("webhook: host lookup failed for %q: %w", host, err)
			}
			for _, a := range addrs {
				ips = append(ips, a.IP)
			}
		}
		if !allowPrivate {
			for _, ip := range ips {
				if isPrivateIP(ip) {
					return nil, fmt.Errorf("webhook: refusing to dial private address %q", host)
				}
			}
		}
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, addr)
	}
}

// NewSafeClient returns an *http.Client that enforces TLS 1.2 minimum,
// refuses private-address dials unless allowPrivate is true, never follows
// redirects (CheckRedirect returns http.ErrUseLastResponse so callers see the
// 3xx response), and applies timeout to the whole request.
func NewSafeClient(timeout time.Duration, allowPrivate bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone() //nolint:forcetypeassert // http.DefaultTransport is always *http.Transport in the standard library.
	transport.DialContext = SafeDialContext(allowPrivate)
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
