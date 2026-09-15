package password_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/password"
)

func TestAdapterString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		adapter password.Adapter
		want    string
	}{
		{name: "argon2id", adapter: password.AdapterArgon2ID, want: "argon2id"},
		{name: "unknown", adapter: password.Adapter(999), want: "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.adapter.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseAdapter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		want    password.Adapter
		wantErr bool
	}{
		{name: "argon2id", input: "argon2id", want: password.AdapterArgon2ID, wantErr: false},
		{name: "empty", input: "", want: password.AdapterArgon2ID, wantErr: true},
		{name: "uppercase", input: "Argon2id", want: password.AdapterArgon2ID, wantErr: true},
		{name: "other", input: "bcrypt", want: password.AdapterArgon2ID, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := password.ParseAdapter(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected non-nil error")
				}

				var iae *password.InvalidAdapterError
				if !errors.As(err, &iae) {
					t.Fatalf("errors.As(%v) to *InvalidAdapterError = false", err)
				}

				if !errors.Is(err, password.ErrInvalidAdapter) {
					t.Fatalf("errors.Is(%v, ErrInvalidAdapter) = false", err)
				}

				if got != tc.want {
					t.Fatalf("adapter = %v, want %v on failure", got, tc.want)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseAdapter(%q) = %v, want nil", tc.input, err)
			}

			if got != tc.want {
				t.Fatalf("ParseAdapter(%q) = %v, want %v", tc.input, got, tc.want)
			}

			if got.String() != tc.input {
				t.Fatalf("roundtrip String() = %q, want %q", got.String(), tc.input)
			}
		})
	}
}
