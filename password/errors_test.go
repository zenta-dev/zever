package password_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/zenta-dev/zever/password"
)

func TestSentinels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want error
	}{
		{
			name: "invalid hash via validate",
			err:  password.Options{}.Validate(),
			want: password.ErrInvalidHash,
		},
		{
			name: "password too long wrapped",
			err:  fmt.Errorf("hash: %w", password.ErrPasswordTooLong),
			want: password.ErrPasswordTooLong,
		},
		{
			name: "nil factory via register",
			err:  mustRegisterNil(t, password.Adapter(201)),
			want: password.ErrNilFactory,
		},
		{
			name: "duplicate via double register",
			err:  mustRegisterDup(t, password.Adapter(202)),
			want: password.ErrDuplicate,
		},
		{
			name: "unknown adapter via open",
			err:  mustOpenUnknown(t, password.Adapter(999)),
			want: password.ErrUnknownAdapter,
		},
		{
			name: "invalid adapter via parse",
			err:  mustParseInvalid("bcrypt"),
			want: password.ErrInvalidAdapter,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.err == nil {
				t.Fatal("expected non-nil error")
			}

			if !errors.Is(tc.err, tc.want) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", tc.err, tc.want)
			}
		})
	}
}

func TestTypedErrorStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "invalid adapter",
			err:  &password.InvalidAdapterError{Adapter: "bcrypt"},
			want: `password: invalid adapter: "bcrypt"`,
		},
		{
			name: "duplicate",
			err:  &password.DuplicateError{Adapter: password.AdapterArgon2ID},
			want: "password: duplicate registration: argon2id",
		},
		{
			name: "unknown adapter",
			err:  &password.UnknownAdapterError{Adapter: password.Adapter(999)},
			want: "password: unknown adapter: unknown",
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

func mustRegisterNil(t *testing.T, a password.Adapter) error {
	t.Helper()

	err := password.Register(a, nil)
	if err == nil {
		t.Fatal("expected non-nil error from nil factory register")
	}

	return err
}

func mustRegisterDup(t *testing.T, a password.Adapter) error {
	t.Helper()

	if err := password.Register(a, stubFactory); err != nil {
		t.Fatalf("first Register() = %v, want nil", err)
	}

	err := password.Register(a, stubFactory)
	if err == nil {
		t.Fatal("expected non-nil error from duplicate register")
	}

	return err
}

func mustOpenUnknown(t *testing.T, a password.Adapter) error {
	t.Helper()

	h, err := password.Open(a, password.Options{})
	if err == nil {
		t.Fatal("expected non-nil error from unknown open")
	}

	if h != nil {
		t.Fatal("expected nil hasher on unknown open")
	}

	return err
}

func mustParseInvalid(s string) error {
	_, err := password.ParseAdapter(s)
	return err
}
