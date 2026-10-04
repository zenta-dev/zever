package posthog

import "testing"

// TestEdgeCheckEndpoint_table covers the endpoint scheme/host boundary:
// https always allowed, http only for loopback (IPv4/IPv6/localhost), and
// everything else rejected before any client is constructed.
func TestEdgeCheckEndpoint_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"empty", "", false},
		{"https", "https://us.i.posthog.com", false},
		{"localhost http", "http://localhost:8000", false},
		{"ipv4 loopback", "http://127.0.0.1:8000", false},
		{"ipv6 loopback", "http://[::1]:8000", false},
		{"public http", "http://example.com", true},
		{"public ip http", "http://8.8.8.8", true},
		{"ftp scheme", "ftp://example.com", true},
		{"no scheme", "example.com", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := checkEndpoint(tc.endpoint)
			if tc.wantErr && err == nil {
				t.Fatalf("checkEndpoint(%q) = nil, want error", tc.endpoint)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("checkEndpoint(%q) = %v, want nil", tc.endpoint, err)
			}
		})
	}
}
