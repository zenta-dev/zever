package ratelimit

import (
	"errors"
	"testing"
)

func TestParseTrustedProxies_valid(t *testing.T) {
	t.Parallel()

	got, err := ParseTrustedProxies([]string{"10.0.0.0/8", " 2001:db8::/32 ", "192.168.1.7", "2001:db8::9"})
	if err != nil {
		t.Fatalf("ParseTrustedProxies() err = %v", err)
	}

	want := []string{"10.0.0.0/8", "2001:db8::/32", "192.168.1.7/32", "2001:db8::9/128"}
	if len(got) != len(want) {
		t.Fatalf("ParseTrustedProxies() = %v, want %v", got, want)
	}

	for i, w := range want {
		if got[i].String() != w {
			t.Fatalf("ParseTrustedProxies()[%d] = %v, want %v", i, got[i], w)
		}
	}
}

func TestParseTrustedProxies_invalid(t *testing.T) {
	t.Parallel()

	for _, entries := range [][]string{
		{"10.0.0.0/33"},
		{"999.0.0.0/8"},
		{"proxy.internal"},
		{"10.0.0.0/8", ""},
		{"   "},
	} {
		if _, err := ParseTrustedProxies(entries); !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("ParseTrustedProxies(%q) err = %v, want ErrInvalidOptions", entries, err)
		}
	}
}

func TestOptions_trustedProxiesValidated(t *testing.T) {
	t.Parallel()

	if err := (Options{Rate: 10, Burst: 5, TrustedProxies: []string{"10.0.0.0/8"}}).Validate(); err != nil {
		t.Fatalf("Validate() err = %v, want nil", err)
	}

	if err := (Options{Rate: 10, Burst: 5, TrustedProxies: []string{"bogus"}}).Validate(); !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate() err = %v, want ErrInvalidOptions", err)
	}
}
