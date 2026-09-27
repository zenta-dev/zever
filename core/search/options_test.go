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
		name    string
		host    string
		wantErr bool
	}{
		{"empty valid", "", false},
		{"localhost valid", "http://localhost:7700", false},
		{"https valid", "https://example.com", false},
		{"path valid", "http://localhost:7700/v1", false},
		{"missing scheme invalid", "localhost:7700", true},
		{"missing host invalid", "http:///path", true},
		{"parse error invalid", "http://[::1", true},
		{"scheme only invalid", "http://", true},
		{"bare word invalid", "bogus", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := (Options{Host: tc.host}).Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate(%q) err = %v, want ErrInvalidOptions", tc.host, err)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q) err = %v, want nil", tc.host, err)
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
