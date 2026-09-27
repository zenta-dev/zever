package webhook

import (
	"net"
	"strings"
	"testing"
)

func TestIsPrivateIP_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ip   net.IP
		want bool
	}{
		{"loopback v4", net.ParseIP("127.0.0.1"), true},
		{"loopback v4 /8", net.ParseIP("127.200.10.1"), true},
		{"loopback v6", net.ParseIP("::1"), true},
		{"ten /8", net.ParseIP("10.0.0.5"), true},
		{"ten broadcast edge", net.ParseIP("10.255.255.255"), true},
		{"172.16 low edge", net.ParseIP("172.16.0.1"), true},
		{"172.31 high edge", net.ParseIP("172.31.255.254"), true},
		{"172.15 below range", net.ParseIP("172.15.0.1"), false},
		{"172.32 above range", net.ParseIP("172.32.0.1"), false},
		{"192.168 /16", net.ParseIP("192.168.1.1"), true},
		{"192.167 not private", net.ParseIP("192.167.1.1"), false},
		{"link-local v4", net.ParseIP("169.254.10.20"), true},
		{"169.253 not link-local", net.ParseIP("169.253.0.1"), false},
		{"link-local v6", net.ParseIP("fe80::1"), true},
		{"unique local fc00", net.ParseIP("fc00::1"), true},
		{"unique local fd00", net.ParseIP("fd12:3456::1"), true},
		{"unspecified v4", net.ParseIP("0.0.0.0"), true},
		{"unspecified v6", net.ParseIP("::"), true},
		{"public v4", net.ParseIP("8.8.8.8"), false},
		{"public v4 cloudflare", net.ParseIP("1.1.1.1"), false},
		{"public v6", net.ParseIP("2001:db8::1"), false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := IsPrivateIP(c.ip); got != c.want {
				t.Errorf("IsPrivateIP(%v) = %v want %v", c.ip, got, c.want)
			}
		})
	}
}

func TestValidateTargetSyntax_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		target  string
		wantErr bool
	}{
		{"empty", "", true},
		{"bad url", "://bad-url", true},
		{"ftp scheme", "ftp://example.com/hook", true},
		{"no scheme", "example.com/hook", true},
		{"no host", "https:///path", true},
		{"valid http", "http://example.com/hook", false},
		{"valid https", "https://example.com/hook", false},
		{"valid ip literal", "https://8.8.8.8/hook", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTargetSyntax(c.target)
			if c.wantErr && err == nil {
				t.Fatalf("ValidateTargetSyntax(%q) = nil, want error", c.target)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("ValidateTargetSyntax(%q) = %v, want nil", c.target, err)
			}
		})
	}
}

func TestValidateTarget_rejects(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		target string
		want   string
	}{
		{"empty", "", "target is empty"},
		{"bad url", "://bad-url", "is not a valid URL"},
		{"http scheme", "http://example.com/hook", "must use https scheme"},
		{"no host", "https:///path", "has no host"},
		{"private literal", "https://127.0.0.1/hook", "resolves to private address"},
		{"private ten", "https://10.1.2.3/hook", "resolves to private address"},
		{"private dns", "https://localhost/hook", "resolves to private address"},
		{"lookup failure", "https://nonexistent.invalid/hook", "host lookup failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTarget(c.target)
			if err == nil {
				t.Fatalf("ValidateTarget(%q) = nil, want error containing %q", c.target, c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("ValidateTarget(%q) = %q, want substring %q", c.target, err.Error(), c.want)
			}
		})
	}
}

func TestValidateTarget_acceptsPublicLiteral(t *testing.T) {
	t.Parallel()
	for _, target := range []string{"https://8.8.8.8/hook", "https://1.1.1.1/hook", "https://[2001:db8::1]/hook"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()
			if err := ValidateTargetContext(t.Context(), target); err != nil {
				t.Errorf("ValidateTargetContext(%q) = %v, want nil", target, err)
			}
		})
	}
}
