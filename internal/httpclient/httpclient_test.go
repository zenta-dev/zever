package httpclient

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewClientTLSFloor(t *testing.T) {
	c := NewClient(5 * time.Second)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("TLS MinVersion = %v, want >= TLS1.2", tr.TLSClientConfig)
	}
	if c.Timeout != 5*time.Second {
		t.Fatalf("Timeout = %v", c.Timeout)
	}
}

func TestReadLimitedOverLimit(t *testing.T) {
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 10))
	_, err := ReadLimited(body, 5)
	if err == nil {
		t.Fatalf("expected over-limit error")
	}
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("errors.Is = false, err = %v", err)
	}
	var tl *TooLargeError
	if !errors.As(err, &tl) {
		t.Fatalf("errors.As TooLargeError failed: %v", err)
	}
}

func TestReadLimitedWithinLimit(t *testing.T) {
	body := bytes.NewReader([]byte("hello"))
	got, err := ReadLimited(body, 10)
	if err != nil {
		t.Fatalf("ReadLimited: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestSafeDialGuardBlocksPrivate(t *testing.T) {
	c := NewSafeClient(time.Second, false)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T", c.Transport)
	}
	if tr.DialContext == nil {
		t.Fatalf("DialContext nil")
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1/", nil)
	_ = req
	// Dial should refuse loopback when allowPrivate=false.
	dial := tr.DialContext
	errCh := make(chan error, 1)
	go func() {
		// Use a short timeout context; SafeDial fails before dialing.
		conn, err := dial(t.Context(), "tcp", "127.0.0.1:80")
		if conn != nil {
			_ = conn.Close()
		}
		errCh <- err
	}()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("expected private dial refusal")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("dial hung")
	}
}

func TestNoRedirectPolicy(t *testing.T) {
	c := NewSafeClient(time.Second, false)
	if c.CheckRedirect == nil {
		t.Fatalf("CheckRedirect nil")
	}
	if err := c.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect = %v", err)
	}
	plain := NewClient(time.Second)
	if plain.CheckRedirect != nil {
		t.Fatalf("plain client should follow redirects by default")
	}
}

func TestReadLimitedReadError(t *testing.T) {
	_, err := ReadLimited(errReader{}, 10)
	if err == nil || errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected non-TooLarge read error, got %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

var _ = io.Discard

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		// Loopback / unspecified / link-local.
		{"ipv4 loopback", "127.0.0.1", true},
		{"ipv6 loopback", "::1", true},
		{"ipv4 unspecified", "0.0.0.0", true},
		{"ipv6 unspecified", "::", true},
		{"ipv4 link-local unicast", "169.254.1.1", true},
		{"ipv6 link-local unicast", "fe80::1", true},
		{"ipv4 link-local multicast", "224.0.0.1", true},

		// RFC1918 private ranges, representative + boundary addresses.
		{"10/8 representative", "10.1.2.3", true},
		{"10/8 first address", "10.0.0.0", true},
		{"10/8 last address", "10.255.255.255", true},
		{"172.16/12 representative", "172.20.1.1", true},
		{"172.16/12 first address", "172.16.0.0", true},
		{"172.16/12 last address", "172.31.255.255", true},
		{"192.168/16 representative", "192.168.1.1", true},
		{"192.168/16 first address", "192.168.0.0", true},
		{"192.168/16 last address", "192.168.255.255", true},

		// IPv6 unique-local fc00::/7.
		{"fc00::/7 fc-representative", "fc00::1", true},
		{"fc00::/7 fd-representative", "fd12:3456:789a::1", true},
		{"fc00::/7 first address", "fc00::", true},
		{"fc00::/7 last address", "fdff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", true},

		// Off-by-one boundaries just outside the private ranges: must be public.
		{"just below 10/8", "9.255.255.255", false},
		{"just above 10/8", "11.0.0.0", false},
		{"just below 172.16/12", "172.15.255.255", false},
		{"just above 172.16/12", "172.32.0.0", false},
		{"just below 192.168/16", "192.167.255.255", false},
		{"just above 192.168/16", "192.169.0.0", false},
		{"just below fc00::/7", "fbff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", false},
		{"just above fc00::/7", "fe00::", false},

		// Clearly public addresses.
		{"public ipv4", "8.8.8.8", false},
		{"public ipv4 alt", "1.1.1.1", false},
		{"public ipv6", "2001:4860:4860::8888", false},
		// Private-looking but not actually private: 172.x outside 16-31.
		{"172.1 not private", "172.1.1.1", false},
		{"192.169 not private", "192.169.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) failed", tt.ip)
			}
			if got := IsPrivateIP(ip); got != tt.want {
				t.Fatalf("IsPrivateIP(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsPrivateIPNil(t *testing.T) {
	if IsPrivateIP(nil) {
		t.Fatalf("IsPrivateIP(nil) = true, want false")
	}
}

func TestWithTransport(t *testing.T) {
	custom := &http.Transport{MaxIdleConns: 7}
	c := NewClient(time.Second, WithTransport(custom))
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}
	if tr == custom {
		t.Fatalf("WithTransport should install a clone, not the original pointer")
	}
	if tr.MaxIdleConns != 7 {
		t.Fatalf("MaxIdleConns = %d, want 7 (custom transport field not carried over)", tr.MaxIdleConns)
	}
	// TLS floor is still applied on top of the custom transport.
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("TLS MinVersion = %v, want >= TLS1.2", tr.TLSClientConfig)
	}
}

func TestWithTransportNonHTTPTransport(t *testing.T) {
	fake := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unused")
	})
	c := NewClient(time.Second, WithTransport(fake))
	if _, ok := c.Transport.(roundTripFunc); !ok {
		t.Fatalf("Transport is %T, want roundTripFunc (used directly, no clone)", c.Transport)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestWithInsecureSkipVerify(t *testing.T) {
	c := NewClient(time.Second, WithInsecureSkipVerify(true))
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("InsecureSkipVerify = %v, want true", tr.TLSClientConfig)
	}
}

func TestWithInsecureSkipVerifyFalse(t *testing.T) {
	c := NewClient(time.Second, WithInsecureSkipVerify(false))
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("InsecureSkipVerify = %v, want false", tr.TLSClientConfig)
	}
}

func TestTooLargeErrorMessage(t *testing.T) {
	tests := []struct {
		name  string
		limit int64
		size  int64
		want  string
	}{
		{"small values", 5, 10, "httpclient: response body too large: 10 bytes exceeds limit 5"},
		{"large values", 1048576, 1048577, "httpclient: response body too large: 1048577 bytes exceeds limit 1048576"},
		{"zero limit", 0, 1, "httpclient: response body too large: 1 bytes exceeds limit 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &TooLargeError{Limit: tt.limit, Size: tt.size}
			if got := err.Error(); got != tt.want {
				t.Fatalf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
