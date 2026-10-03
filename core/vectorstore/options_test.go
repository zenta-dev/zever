package vectorstore

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

func TestOptions_Validate_negativeDimension(t *testing.T) {
	t.Parallel()

	err := (Options{Dimension: -1}).Validate()
	if !errors.Is(err, ErrInvalidOptions) {
		t.Fatalf("Validate err = %v, want ErrInvalidOptions", err)
	}

	var ioe *InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("err %T is not *InvalidOptionsError", err)
	}
}

func TestOptions_Validate_urls(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		url           string
		allowInsecure bool
		wantErr       bool
	}{
		{"empty valid", "", false, false},
		{"https valid", "https://qdrant.example.com:6334", false, false},
		{"http insecure invalid by default", "http://localhost:6334", false, true},
		{"http insecure allowed", "http://localhost:6334", true, false},
		{"missing scheme invalid", "localhost:6334", false, true},
		{"missing host invalid", "https://", false, true},
		{"scheme only invalid", "http:", false, true},
		{"control char invalid", "http://example.com/\x7f", false, true},
		{"unsupported scheme invalid", "grpc://example.com:6334", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := (Options{URL: tc.url, AllowInsecure: tc.allowInsecure}).Validate()
			if tc.wantErr && !errors.Is(err, ErrInvalidOptions) {
				t.Fatalf("Validate(%q, insecure=%v) err = %v, want ErrInvalidOptions", tc.url, tc.allowInsecure, err)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q, insecure=%v) err = %v, want nil", tc.url, tc.allowInsecure, err)
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

func TestOptions_Validate_joinedMultiples(t *testing.T) {
	t.Parallel()

	err := (Options{Dimension: -1, URL: "localhost:6334"}).Validate()
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

func TestOptions_consts(t *testing.T) {
	t.Parallel()

	if DefaultDimension != 1536 {
		t.Errorf("DefaultDimension = %d, want 1536", DefaultDimension)
	}

	if DefaultTopK != 10 {
		t.Errorf("DefaultTopK = %d, want 10", DefaultTopK)
	}

	if DefaultSQLiteDSN != ":memory:" {
		t.Errorf("DefaultSQLiteDSN = %q, want %q", DefaultSQLiteDSN, ":memory:")
	}
}
