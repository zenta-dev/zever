package password_test

import (
	"testing"

	"github.com/zenta-dev/zever/core/password"
)

func TestTypedErrorStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "invalid adapter",
			err:  password.InvalidAdapterError{Adapter: "bcrypt"},
			want: `password: invalid adapter: "bcrypt"`,
		},
		{
			name: "duplicate",
			err:  password.DuplicateAdapterError{Adapter: "argon2id"},
			want: "password: duplicate adapter: argon2id",
		},
		{
			name: "unknown adapter",
			err:  password.UnknownAdapterError{Adapter: "unknown"},
			want: "password: unknown adapter: unknown (forgotten import?)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

