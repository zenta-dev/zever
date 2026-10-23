package ratelimit

import (
	"errors"
	"strings"
	"testing"
)

// TestRegRemovalTypedErrors covers every typed error left uncovered when the
// adapter registry was removed: Register/Open used to exercise the adapter
// errors via duplicate/unknown/invalid paths, so direct unit tests now pin
// the message constructors, sentinel unwrapping, and As round-trips. Key
// material is never echoed, so the key case asserts the length only.
func TestRegRemovalTypedErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		err      error
		sentinel error
		wantSub  string
		asTarget any
	}{
		{"duplicate", DuplicateAdapterError{Adapter: "redis"}, ErrDuplicateAdapter, "redis", &DuplicateAdapterError{}},
		{"unknown", UnknownAdapterError{Adapter: "redis"}, ErrUnknownAdapter, "redis", &UnknownAdapterError{}},
		{"invalid", InvalidAdapterError{Adapter: "bogus"}, ErrInvalidAdapter, "bogus", &InvalidAdapterError{}},
		{"options", InvalidOptionsError{Reason: "bad rate"}, ErrInvalidOptions, "bad rate", &InvalidOptionsError{}},
		{"key", InvalidKeyError{KeyLen: 9}, ErrInvalidKey, "9", &InvalidKeyError{}},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			msg := tc.err.Error()
			if !strings.HasPrefix(msg, "ratelimit: ") {
				t.Errorf("Error() = %q, want prefix %q", msg, "ratelimit: ")
			}

			if !strings.Contains(msg, tc.wantSub) {
				t.Errorf("Error() = %q, want substring %q", msg, tc.wantSub)
			}

			if !errors.Is(tc.err, tc.sentinel) {
				t.Errorf("errors.Is(err, sentinel) = false (err = %v)", tc.err)
			}

			if !errors.As(tc.err, tc.asTarget) {
				t.Errorf("errors.As(%T) = false", tc.err)
			}
		})
	}
}

// TestRegRemovalUnmapPrefix4In6 covers the IPv4-mapped IPv6 prefix rewrite:
// ::ffff:a.b.c.d/n (n >= 96) collapses to its plain IPv4 form so mapped
// trusted-proxy CIDRs match unmapped client addresses.
func TestRegRemovalUnmapPrefix4In6(t *testing.T) {
	t.Parallel()

	got, err := ParseTrustedProxies([]string{"::ffff:10.1.2.3/120"})
	if err != nil {
		t.Fatalf("ParseTrustedProxies() err = %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("ParseTrustedProxies() = %v, want one prefix", got)
	}

	if got[0].String() != "10.1.2.0/24" {
		t.Errorf("ParseTrustedProxies() = %v, want 10.1.2.0/24", got)
	}
}
