package search

import (
	"errors"
	"testing"
)

func TestOptions_Validate_zero_ok(t *testing.T) {
	t.Parallel()

	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate zero err = %v, want nil", err)
	}
}

func TestOptions_Validate_hosts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		host          string
		allowInsecure bool
		wantErr       bool
	}{
		{"empty valid", "", false, false},
		{"https valid", "https://example.com", false, false},
		{"https path valid", "https://example.com/v1", false, false},
		{"http insecure invalid by default", "http://localhost:7700", false, true},
		{"http insecure allowed", "http://localhost:7700", true, false},
		{"http insecure path allowed", "http://localhost:7700/v1", true, false},
		{"missing scheme invalid", "localhost:7700", false, true},
		{"missing host invalid", "https:///path", false, true},
		{"parse error invalid", "https://[::1", false, true},
		{"scheme only invalid", "https://", false, true},
		{"bare word invalid", "bogus", false, true},
		{"unsupported scheme invalid", "ftp://example.com", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := (Options{Host: tc.host, AllowInsecure: tc.allowInsecure}).Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate(%q, insecure=%v) err = %v, want ErrInvalidOptions", tc.host, tc.allowInsecure, err)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q, insecure=%v) err = %v, want nil", tc.host, tc.allowInsecure, err)
			}

			if tc.wantErr {
				var ioe *InvalidOptionsError
				if !errors.As(err, &ioe) {
					t.Fatalf("err %T is not *InvalidOptionsError", err)
				}
			}
		})
	}
}

func TestOptions_Validate_joined(t *testing.T) {
	t.Parallel()

	err := (Options{Host: "bogus"}).Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
	}

	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("err %T does not unwrap to []error, want joined errors", err)
	}

	if got := len(joined.Unwrap()); got != 1 {
		t.Fatalf("joined errors = %d, want 1", got)
	}
}
