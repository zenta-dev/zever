package tenant

import (
	"errors"
	"strings"
	"testing"
)

func TestOptions_Validate_zero_ok(t *testing.T) {
	t.Parallel()

	if err := (Options{}).Validate(); err != nil {
		t.Fatalf("Validate zero err = %v, want nil", err)
	}
}

func TestOptions_Validate_headers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		header  string
		wantErr bool
	}{
		{"empty valid", "", false},
		{"default valid", DefaultHeader, false},
		{"simple valid", "X-Custom", false},
		{"space invalid", "X Bad", true},
		{"colon invalid", "X:Bad", true},
		{"control invalid", "X\x01Bad", true},
		{"del invalid", "X\x7fBad", true},
		{"non-ascii invalid", "X-Bäd", true},
		{"tab invalid", "X\tBad", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := (Options{Header: tc.header}).Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate(%q) err = %v, want ErrInvalidOptions", tc.header, err)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q) err = %v, want nil", tc.header, err)
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

func TestOptions_Validate_regexTooLong(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", MaxRegexLength+1)
	err := (Options{SubdomainRegex: long}).Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate long regex err = %v, want ErrInvalidOptions", err)
	}

	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
}

func TestOptions_Validate_regexAtLimit_ok(t *testing.T) {
	t.Parallel()

	atLimit := strings.Repeat("a", MaxRegexLength)
	if err := (Options{SubdomainRegex: atLimit}).Validate(); err != nil {
		t.Fatalf("Validate at-limit err = %v, want nil", err)
	}
}

func TestOptions_Validate_joinedMultiples(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", MaxRegexLength+1)
	err := (Options{Header: "X Bad", SubdomainRegex: long}).Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate multiples err = %v, want ErrInvalidOptions", err)
	}

	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("err %T does not unwrap to []error, want joined errors", err)
	}

	if got := len(joined.Unwrap()); got != 2 {
		t.Fatalf("joined errors = %d, want 2", got)
	}
}

func TestOptions_Validate_longInvalidRegex_notCompiled(t *testing.T) {
	t.Parallel()

	// Core Validate checks length only; an over-long invalid pattern must
	// still surface as InvalidOptions (length), never a compile error.
	longInvalid := strings.Repeat("[", MaxRegexLength+1)
	err := (Options{SubdomainRegex: longInvalid}).Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
	}
}

func TestValidHeaderKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		key   string
		valid bool
	}{
		{"empty", "", false},
		{"default", DefaultHeader, true},
		{"colon", "X:Bad", false},
		{"space", "X Bad", false},
		{"control", "X\x01", false},
		{"del", "X\x7f", false},
		{"printable", "X-Tenant-ID_123.~!#$", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := validHeaderKey(tc.key); got != tc.valid {
				t.Errorf("validHeaderKey(%q) = %v, want %v", tc.key, got, tc.valid)
			}
		})
	}
}

func TestOptions_consts(t *testing.T) {
	t.Parallel()

	if DefaultHeader != "X-Tenant-ID" {
		t.Errorf("DefaultHeader = %q, want %q", DefaultHeader, "X-Tenant-ID")
	}

	if DefaultSingleID != "default" {
		t.Errorf("DefaultSingleID = %q, want %q", DefaultSingleID, "default")
	}

	if MaxRegexLength != 500 {
		t.Errorf("MaxRegexLength = %d, want 500", MaxRegexLength)
	}
}
