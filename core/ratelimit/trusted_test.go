package ratelimit

import (
	"errors"
	"strings"
	"testing"
)

func TestParseTrustedProxies_valid(t *testing.T) {
	t.Parallel()

	prefixes, err := ParseTrustedProxies([]string{"10.0.0.0/8", "192.168.1.7", "2001:db8::/32", " ::1 "})
	if err != nil {
		t.Fatalf("ParseTrustedProxies: %v", err)
	}
	if len(prefixes) != 4 {
		t.Fatalf("len(prefixes) = %d, want 4", len(prefixes))
	}

	if got := prefixes[1].String(); got != "192.168.1.7/32" {
		t.Errorf("bare IPv4 = %q, want %q", got, "192.168.1.7/32")
	}
	if !prefixes[0].Addr().Is4() || prefixes[2].Addr().Is6() != true {
		t.Errorf("unexpected families: %v", prefixes)
	}
}

func TestParseTrustedProxies_empty(t *testing.T) {
	t.Parallel()

	prefixes, err := ParseTrustedProxies(nil)
	if err != nil {
		t.Fatalf("ParseTrustedProxies(nil): %v", err)
	}
	if len(prefixes) != 0 {
		t.Fatalf("len(prefixes) = %d, want 0", len(prefixes))
	}
}

func TestParseTrustedProxies_invalid(t *testing.T) {
	t.Parallel()

	_, err := ParseTrustedProxies([]string{"10.0.0.0/8", "not-a-cidr"})
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "not-a-cidr") {
		t.Fatalf("err = %q, want it to name the bad entry", err)
	}
}

func TestOptions_badTrustedProxy_isInvalid(t *testing.T) {
	t.Parallel()

	o := Options{Rate: 10, Burst: 5, TrustedProxies: []string{"10.0.0.0/33"}}
	if err := o.Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestOptions_goodTrustedProxies_areValid(t *testing.T) {
	t.Parallel()

	o := Options{Rate: 10, Burst: 5, TrustedProxies: []string{"10.0.0.0/8", "2001:db8::/32"}}
	if err := o.Validate(); err != nil {
		t.Fatalf("Validate err = %v, want nil", err)
	}
}
