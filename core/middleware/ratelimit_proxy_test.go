package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/zenta-dev/zever/core/ratelimit"
)

// mustPrefixes parses CIDRs or fails the test.
func mustPrefixes(t *testing.T, entries ...string) []netip.Prefix {
	t.Helper()

	prefixes, err := ParseTrustedProxies(entries)
	if err != nil {
		t.Fatalf("ParseTrustedProxies(%q) err = %v", entries, err)
	}

	return prefixes
}

// proxyRequest builds a request from peer RemoteAddr with an X-Forwarded-For header.
func proxyRequest(t *testing.T, remoteAddr, xff string) *http.Request {
	t.Helper()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}

	return r
}

func TestClientIPKey(t *testing.T) {
	trusted := mustPrefixes(t, "10.0.0.0/8", "fd00::/8")

	tests := []struct {
		name       string
		trust      []netip.Prefix
		remoteAddr string
		xff        string
		want       string
	}{
		{"spoofed XFF from untrusted peer ignored", trusted, "203.0.113.9:5555", "1.2.3.4", "203.0.113.9"},
		{"untrusted peer without XFF uses remote", trusted, "203.0.113.9:5555", "", "203.0.113.9"},
		{"trusted peer honors client before proxy", trusted, "10.0.0.2:5555", "1.2.3.4, 10.0.0.1", "1.2.3.4"},
		{"rightmost untrusted wins over spoofed left", trusted, "10.0.0.2:5555", "9.9.9.9, 8.8.8.8, 10.0.0.1", "8.8.8.8"},
		{"all trusted falls back to leftmost client", trusted, "10.0.0.2:5555", "10.1.0.1, 10.0.0.1", "10.1.0.1"},
		{"all trusted multi-hop returns origin", trusted, "10.0.0.9:5555", "10.9.9.9, 10.0.0.1, 10.0.0.2", "10.9.9.9"},
		{"ipv6 client behind trusted proxy", trusted, "[fd00::2]:443", "2001:db8::5, fd00::1", "2001:db8::5"},
		{"ipv6 peer untrusted ignores XFF", trusted, "[2001:db8::9]:443", "1.2.3.4", "2001:db8::9"},
		{"malformed entries skipped", trusted, "10.0.0.2:5555", "garbage, , 1.2.3.4", "1.2.3.4"},
		{"fully malformed header falls back to remote", trusted, "10.0.0.2:5555", "!!!", "10.0.0.2"},
		{"empty trust list always uses remote", nil, "10.0.0.2:5555", "1.2.3.4", "10.0.0.2"},
		{"hostport entry tolerated", trusted, "10.0.0.2:5555", "1.2.3.4:5678, 10.0.0.1", "1.2.3.4"},
		{"unparsable peer is untrusted", trusted, "not-an-addr", "1.2.3.4", "not-an-addr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := ClientIPKey(tt.trust)
			if got := key(proxyRequest(t, tt.remoteAddr, tt.xff)); got != tt.want {
				t.Fatalf("ClientIPKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		want    []string
		wantErr bool
	}{
		{"cidrs", []string{"10.0.0.0/8", "2001:db8::/32"}, []string{"10.0.0.0/8", "2001:db8::/32"}, false},
		{"bare ipv4 becomes /32", []string{"10.1.2.3"}, []string{"10.1.2.3/32"}, false},
		{"bare ipv6 becomes /128", []string{"2001:db8::1"}, []string{"2001:db8::1/128"}, false},
		{"empty list", nil, nil, false},
		{"blank entry fails", []string{"10.0.0.0/8", " "}, nil, true},
		{"garbage fails", []string{"not-a-cidr"}, nil, true},
		{"hostname fails", []string{"proxy.internal"}, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTrustedProxies(tt.entries)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseTrustedProxies() err = nil, want error")
				}

				if !errors.Is(err, ratelimit.ErrInvalidOptions) {
					t.Fatalf("ParseTrustedProxies() err = %v, want ErrInvalidOptions", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseTrustedProxies() err = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("ParseTrustedProxies() = %v, want %v", got, tt.want)
			}

			for i, want := range tt.want {
				if got[i].String() != want {
					t.Fatalf("ParseTrustedProxies()[%d] = %v, want %v", i, got[i], want)
				}
			}
		})
	}
}

// multiHeaderRequest builds a request with one X-Forwarded-For line per value.
func multiHeaderRequest(t *testing.T, remoteAddr string, lines ...string) *http.Request {
	t.Helper()

	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr

	for _, l := range lines {
		r.Header.Add("X-Forwarded-For", l)
	}

	return r
}

func TestClientIPKey_multipleHeaderLines(t *testing.T) {
	trusted := mustPrefixes(t, "10.0.0.0/8", "::ffff:192.168.0.0/112")

	tests := []struct {
		name       string
		remoteAddr string
		lines      []string
		want       string
	}{
		{"proxy-appended line wins over spoofed first line", "10.0.0.2:1", []string{"6.6.6.6", "8.8.8.8"}, "8.8.8.8"},
		{"spoofed leading entry with trailing trusted entry", "10.0.0.2:1", []string{"6.6.6.6", "8.8.8.8, 10.0.0.1"}, "8.8.8.8"},
		{"trusted last line skipped", "10.0.0.2:1", []string{"6.6.6.6", "8.8.8.8", "10.0.0.1"}, "8.8.8.8"},
		{"untrusted peer ignores every line", "203.0.113.9:1", []string{"6.6.6.6", "8.8.8.8"}, "203.0.113.9"},
		{"ipv4-mapped trusted cidr matches ipv4 entry", "192.168.0.5:1", []string{"6.6.6.6", "8.8.8.8, 192.168.0.9"}, "8.8.8.8"},
		{"ipv4-mapped peer matches ipv4 cidr", "[::ffff:10.0.0.2]:1", []string{"6.6.6.6", "8.8.8.8"}, "8.8.8.8"},
		{"ipv6 client across lines", "10.0.0.2:1", []string{"6.6.6.6", "2001:db8::5"}, "2001:db8::5"},
		{"malformed lines fall back to peer", "10.0.0.2:1", []string{"garbage", "!!!"}, "10.0.0.2"},
		{"empty lines fall back to peer", "10.0.0.2:1", []string{"", " "}, "10.0.0.2"},
		{"malformed last line skipped", "10.0.0.2:1", []string{"6.6.6.6, 8.8.8.8", "garbage"}, "8.8.8.8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClientIPKey(trusted)(multiHeaderRequest(t, tt.remoteAddr, tt.lines...))
			if got != tt.want {
				t.Fatalf("ClientIPKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTrustedProxies_unmapsIPv4Mapped(t *testing.T) {
	got := mustPrefixes(t, "::ffff:10.0.0.0/104", "::ffff:1.2.3.4")

	want := []string{"10.0.0.0/8", "1.2.3.4/32"}
	for i, p := range got {
		if p.String() != want[i] {
			t.Fatalf("prefix[%d] = %s, want %s", i, p, want[i])
		}
	}
}
