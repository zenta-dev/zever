package log

import (
	"errors"
	"testing"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()

	cases := map[string][2]string{
		"ErrNilFactory":     {ErrNilFactory.Error(), "log: nil factory"},
		"ErrDuplicate":      {ErrDuplicate.Error(), "log: duplicate adapter"},
		"ErrUnknownAdapter": {ErrUnknownAdapter.Error(), "log: unknown adapter"},
		"ErrInvalidLevel":   {ErrInvalidLevel.Error(), "log: invalid level"},
		"ErrInvalidAdapter": {ErrInvalidAdapter.Error(), "log: invalid adapter"},
	}

	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestParseLevelInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseLevel("bogus")
	if err == nil {
		t.Fatal("ParseLevel(bogus) = nil, want ErrInvalidLevel")
	}

	if !errors.Is(err, ErrInvalidLevel) {
		t.Errorf("errors.Is(err, ErrInvalidLevel) = false (err = %v)", err)
	}
}

func TestTypedErrors_messageAndUnwrap(t *testing.T) {
	t.Parallel()

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		err := DuplicateAdapterError{Adapter: "noop"}
		if got, want := err.Error(), `log: duplicate adapter: noop`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, ErrDuplicate) {
			t.Errorf("errors.Is(err, ErrDuplicate) = false (err = %v)", err)
		}
	})

	t.Run("unknown adapter", func(t *testing.T) {
		t.Parallel()

		err := UnknownAdapterError{Adapter: "unknown"}
		if got, want := err.Error(), `log: unknown adapter: unknown (forgotten import?)`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, ErrUnknownAdapter) {
			t.Errorf("errors.Is(err, ErrUnknownAdapter) = false (err = %v)", err)
		}
	})

	t.Run("invalid level", func(t *testing.T) {
		t.Parallel()

		err := InvalidLevelError{Level: "bogus"}
		if got, want := err.Error(), `log: invalid level: "bogus"`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, ErrInvalidLevel) {
			t.Errorf("errors.Is(err, ErrInvalidLevel) = false (err = %v)", err)
		}
	})

	t.Run("invalid adapter", func(t *testing.T) {
		t.Parallel()

		err := InvalidAdapterError{Adapter: "bogus"}
		if got, want := err.Error(), `log: invalid adapter: "bogus"`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, ErrInvalidAdapter) {
			t.Errorf("errors.Is(err, ErrInvalidAdapter) = false (err = %v)", err)
		}
	})
}
