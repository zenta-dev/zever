package httpclient

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestIsPrivateIP_ipv6Embedded verifies transition-mechanism IPv6 forms are
// classified by their embedded IPv4 address.
func TestIsPrivateIP_ipv6Embedded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"nat64 metadata", "64:ff9b::a9fe:a9fe", true},
		{"nat64 10/8", "64:ff9b::a00:1", true},
		{"nat64 loopback", "64:ff9b::7f00:1", true},
		{"nat64 cgnat", "64:ff9b::6440:1", true},
		{"nat64 public", "64:ff9b::808:808", false},
		{"nat64 local-use metadata", "64:ff9b:1::a9fe:a9fe", true},
		{"nat64 local-use 10/8", "64:ff9b:1::a00:1", true},
		{"nat64 local-use public", "64:ff9b:1::808:808", false},
		{"6to4 metadata", "2002:a9fe:a9fe::1", true},
		{"6to4 10/8", "2002:a00:1::1", true},
		{"6to4 loopback", "2002:7f00:1::1", true},
		{"6to4 cgnat", "2002:6440:1::1", true},
		{"6to4 public", "2002:808:808::1", false},
		{"teredo", "2001:0:4136:e378:8000:63bf:3fff:fdd2", true},
		{"teredo zero", "2001::1", true},
		{"site-local", "fec0::1", true},
		{"site-local upper", "feff::1", true},
		{"v4-compatible metadata", "::a9fe:a9fe", true},
		{"v4-compatible 10/8", "::a00:1", true},
		{"v4-compatible loopback", "::7f00:1", true},
		{"v4-compatible cgnat", "::6440:1", true},
		{"v4-compatible public", "::808:808", false},
		{"v4-mapped metadata", "::ffff:169.254.169.254", true},
		{"v4-mapped public", "::ffff:8.8.8.8", false},
		{"doc range stays dialable", "2001:db8::1", false},
		{"public v6", "2606:4700::1111", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("bad test IP %q", tt.ip)
			}
			if got := IsPrivateIP(ip); got != tt.want {
				t.Errorf("IsPrivateIP(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// TestNewSafeClient_ignoresProxyEnv verifies the safe client never routes
// through an environment proxy, which would bypass the target-address guard.
func TestNewSafeClient_ignoresProxyEnv(t *testing.T) {
	var hits atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("http_proxy", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")

	c := NewSafeClient(2*time.Second, false, WithLookup(stubLookup(t, "10.0.0.5")))
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://internal.example.test/", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected private target to be refused")
	}
	if hits.Load() != 0 {
		t.Fatalf("request reached proxy %d times; proxy must be bypassed", hits.Load())
	}

	tr, ok := c.Transport.(*http.Transport)
	if !ok || tr.Proxy != nil {
		t.Fatalf("safe transport Proxy must be nil, got %T", c.Transport)
	}
}

// TestWithAllowProxy_keepsEnvProxy verifies the explicit opt-in restores
// environment proxy resolution.
func TestWithAllowProxy_keepsEnvProxy(t *testing.T) {
	c := NewSafeClient(time.Second, false, WithAllowProxy())
	tr, ok := c.Transport.(*http.Transport)
	if !ok || tr.Proxy == nil {
		t.Fatal("WithAllowProxy must keep Transport.Proxy set")
	}
}
