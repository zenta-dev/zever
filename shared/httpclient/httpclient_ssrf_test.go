package httpclient

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// stubLookup maps hostnames to canned addresses without touching DNS.
func stubLookup(t *testing.T, ips ...string) LookupFunc {
	t.Helper()

	parsed := make([]net.IPAddr, 0, len(ips))
	for _, s := range ips {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("stubLookup: invalid IP %q", s)
		}
		parsed = append(parsed, net.IPAddr{IP: ip})
	}

	return func(_ context.Context, _ string) ([]net.IPAddr, error) {
		return parsed, nil
	}
}

// TestIsPrivateIP_blocksSensitiveRanges verifies link-local, metadata,
// carrier-grade NAT, multicast, reserved, and documentation addresses are
// treated as private while public addresses pass.
func TestIsPrivateIP_blocksSensitiveRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "cloud metadata", ip: "169.254.169.254", want: true},
		{name: "link-local", ip: "169.254.0.1", want: true},
		{name: "unique-local fd00", ip: "fd00::1", want: true},
		{name: "unique-local fc00", ip: "fc00::1", want: true},
		{name: "carrier-grade NAT", ip: "100.64.0.1", want: true},
		{name: "ipv4 multicast", ip: "239.255.255.250", want: true},
		{name: "ipv6 multicast", ip: "ff02::1", want: true},
		{name: "reserved", ip: "240.0.0.1", want: true},
		{name: "current network", ip: "0.0.0.1", want: true},
		{name: "documentation v4 (unroutable public example)", ip: "192.0.2.1", want: false},
		{name: "documentation v4 test-net-2", ip: "198.51.100.1", want: false},
		{name: "documentation v4 test-net-3", ip: "203.0.113.1", want: false},
		{name: "benchmark", ip: "198.18.0.1", want: true},
		{name: "documentation v6 (unroutable public example)", ip: "2001:db8::1", want: false},
		{name: "loopback v4", ip: "127.0.0.1", want: true},
		{name: "loopback v6", ip: "::1", want: true},
		{name: "rfc1918 10", ip: "10.0.0.1", want: true},
		{name: "public v4", ip: "8.8.8.8", want: false},
		{name: "public v4 cloudflare", ip: "1.1.1.1", want: false},
		{name: "public v6", ip: "2001:4860:4860::8888", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := IsPrivateIP(net.ParseIP(tt.ip)); got != tt.want {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// TestSafeDialContextWithLookup_blocksRebindingMix verifies every resolved
// address is checked: a lookup returning one public and one private address
// (the DNS-rebinding shape) is refused, and the resolver runs exactly once
// so there is no check-then-resolve-again TOCTOU.
func TestSafeDialContextWithLookup_blocksRebindingMix(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	lookup := func(_ context.Context, _ string) ([]net.IPAddr, error) {
		calls.Add(1)
		return []net.IPAddr{
			{IP: net.ParseIP("93.184.216.34")},
			{IP: net.ParseIP("169.254.169.254")},
		}, nil
	}

	dial := SafeDialContextWithLookup(false, lookup)
	if _, err := dial(t.Context(), "tcp", "rebind.test:80"); !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("dial error = %v, want ErrPrivateAddress", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("lookup calls = %d, want exactly 1", got)
	}
}

// TestSafeDialContextWithLookup_blocksMetadata verifies a hostname resolving
// only to the cloud metadata address is refused before any dial happens.
func TestSafeDialContextWithLookup_blocksMetadata(t *testing.T) {
	t.Parallel()

	dial := SafeDialContextWithLookup(false, stubLookup(t, "169.254.169.254"))
	if _, err := dial(t.Context(), "tcp", "metadata.test:80"); !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("dial error = %v, want ErrPrivateAddress", err)
	}
}

// TestSafeDialContextWithLookup_nilLookupUsesDefault verifies a nil lookup
// falls back to the system resolver (loopback literal needs no DNS).
func TestSafeDialContextWithLookup_nilLookupUsesDefault(t *testing.T) {
	t.Parallel()

	dial := SafeDialContextWithLookup(false, nil)
	if _, err := dial(t.Context(), "tcp", "127.0.0.1:80"); !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("dial error = %v, want ErrPrivateAddress", err)
	}
}

// TestSafeDialContext_dialsValidatedIP verifies a passing guard dials the
// validated IP directly instead of re-resolving the hostname (which would
// reopen the rebinding window between check and dial).
func TestSafeDialContext_dialsValidatedIP(t *testing.T) {
	t.Parallel()

	var gotAddr string
	stubDial := func(_ context.Context, _, addr string) (net.Conn, error) {
		gotAddr = addr
		return nil, errStubDial
	}

	dial := safeDialContext(false, stubLookup(t, "93.184.216.34"), stubDial)
	if _, err := dial(t.Context(), "tcp", "example.test:80"); !errors.Is(err, errStubDial) {
		t.Fatalf("dial error = %v, want errStubDial (guard must pass)", err)
	}

	if gotAddr != "93.184.216.34:80" {
		t.Fatalf("dialed %q, want validated IP %q", gotAddr, "93.184.216.34:80")
	}
}

// errStubDial marks the stub dialer path in TestSafeDialContext_dialsValidatedIP.
var errStubDial = errors.New("httpclient test: stub dial reached")

// redirectPair builds a redirector pointing at a counting target.
func redirectPair(t *testing.T, location func(target string) string) (target *httptest.Server, redirector *httptest.Server, hits *atomic.Int32) {
	t.Helper()

	hits = &atomic.Int32{}
	target = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	redirector = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, location(target.URL), http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	return target, redirector, hits
}

// TestNewSafeClient_neverFollowsRedirects verifies a redirect to a private
// address is never followed: the 3xx response is returned as-is and the
// target is never contacted.
func TestNewSafeClient_neverFollowsRedirects(t *testing.T) {
	t.Parallel()

	_, redirector, hits := redirectPair(t, func(target string) string { return target + "/final" })

	c := NewSafeClient(5*time.Second, true)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, redirector.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do error = %v, want nil (3xx returned, not followed)", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusFound)
	}

	if got := hits.Load(); got != 0 {
		t.Fatalf("target hits = %d, want 0 (redirect must not be followed)", got)
	}
}

// TestNewSafeClient_redirectGuardedDial verifies that even the initial
// request honors the dial guard: a private target is refused outright.
func TestNewSafeClient_redirectGuardedDial(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := NewSafeClient(5*time.Second, false)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest error = %v", err)
	}
	resp, err := c.Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("Do error = %v, want ErrPrivateAddress", err)
	}
}
