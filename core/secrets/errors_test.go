package secrets_test

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()

	cases := map[string][2]string{
		"ErrNotFound":       {secrets.ErrNotFound.Error(), "secrets: not found"},
		"ErrNotSupported":   {secrets.ErrNotSupported.Error(), "secrets: not supported"},
		"ErrInvalidKey":     {secrets.ErrInvalidKey.Error(), "secrets: invalid key"},
		"ErrNilFactory":     {secrets.ErrNilFactory.Error(), "secrets: nil factory"},
		"ErrDuplicate":      {secrets.ErrDuplicate.Error(), "secrets: duplicate registration"},
		"ErrUnknownAdapter": {secrets.ErrUnknownAdapter.Error(), "secrets: unknown adapter"},
		"ErrInvalidAdapter": {secrets.ErrInvalidAdapter.Error(), "secrets: invalid adapter"},
	}

	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestTypedErrorMessages_unwrap(t *testing.T) {
	t.Parallel()

	t.Run("invalid adapter", func(t *testing.T) {
		t.Parallel()

		err := secrets.InvalidAdapterError{Adapter: "bogus"}
		if got, want := err.Error(), `secrets: invalid adapter: "bogus"`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, secrets.ErrInvalidAdapter) {
			t.Error("errors.Is(err, ErrInvalidAdapter) = false")
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		err := &secrets.DuplicateAdapterError{Adapter: secrets.Env}
		if got, want := err.Error(), `secrets: duplicate registration: env`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, secrets.ErrDuplicate) {
			t.Error("errors.Is(err, ErrDuplicate) = false")
		}
	})

	t.Run("unknown adapter", func(t *testing.T) {
		t.Parallel()

		err := &secrets.UnknownAdapterError{Adapter: secrets.Adapter("")}
		if got, want := err.Error(), `secrets: unknown adapter: unknown (forgotten import?)`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, secrets.ErrUnknownAdapter) {
			t.Error("errors.Is(err, ErrUnknownAdapter) = false")
		}
	})
}
