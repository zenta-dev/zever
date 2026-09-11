package log

import (
	"errors"
	"testing"
)

func TestSentinelMessages(t *testing.T) {
	t.Parallel()

	cases := map[string][2]string{
		"ErrNilFactory":     {ErrNilFactory.Error(), "log: nil factory"},
		"ErrDuplicate":      {ErrDuplicate.Error(), "log: duplicate registration"},
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

func TestRegisterNilFactory(t *testing.T) {
	t.Parallel()

	err := Register(Noop, nil)
	if err == nil {
		t.Fatal("Register(nil) = nil, want ErrNilFactory")
	}

	if !errors.Is(err, ErrNilFactory) {
		t.Errorf("errors.Is(err, ErrNilFactory) = false (err = %v)", err)
	}
}

func TestOpenUnknownAdapter(t *testing.T) {
	t.Parallel()

	_, err := Open(Adapter(99), Options{})
	if err == nil {
		t.Fatal("Open(unknown) = nil, want ErrUnknownAdapter")
	}

	if !errors.Is(err, ErrUnknownAdapter) {
		t.Errorf("errors.Is(err, ErrUnknownAdapter) = false (err = %v)", err)
	}

	var unknownErr *UnknownAdapterError
	if !errors.As(err, &unknownErr) {
		t.Errorf("errors.As(err, UnknownAdapterError) = false (err = %T %v)", err, err)
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

func TestParseAdapterInvalid(t *testing.T) {
	t.Parallel()

	_, err := ParseAdapter("bogus")
	if err == nil {
		t.Fatal("ParseAdapter(bogus) = nil, want ErrInvalidAdapter")
	}

	if !errors.Is(err, ErrInvalidAdapter) {
		t.Errorf("errors.Is(err, ErrInvalidAdapter) = false (err = %v)", err)
	}
}

func TestOpenWrapsFactoryError(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("boom")
	adapter := Adapter(100)

	if err := Register(adapter, func(Options) (Logger, error) { return nil, sentinel }); err != nil {
		if !errors.Is(err, ErrDuplicate) {
			t.Skipf("adapter %d already registered, skipping wrap test", int(adapter))
		}
	}

	_, err := Open(adapter, Options{})
	if err == nil {
		t.Fatal("Open() = nil, want wrapped factory error")
	}

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, sentinel) = false (err = %v)", err)
	}
}

func TestRegisterDuplicate_returnsDuplicateError(t *testing.T) {
	adapter := freshAdapter()

	stub := func(Options) (Logger, error) { return stubLogger{}, nil }
	if err := Register(adapter, stub); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	err := Register(adapter, stub)
	if err == nil {
		t.Fatal("Register(dup) = nil, want ErrDuplicate")
	}

	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("errors.Is(err, ErrDuplicate) = false (err = %v)", err)
	}

	var dupErr *DuplicateError
	if !errors.As(err, &dupErr) {
		t.Fatalf("errors.As(err, DuplicateError) = false (err = %T %v)", err, err)
	}

	if dupErr.Adapter != adapter {
		t.Errorf("DuplicateError.Adapter = %v, want %v", dupErr.Adapter, adapter)
	}
}

func TestTypedErrors_messageAndUnwrap(t *testing.T) {
	t.Parallel()

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		err := &DuplicateError{Adapter: Noop}
		if got, want := err.Error(), `log: duplicate registration: noop`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, ErrDuplicate) {
			t.Errorf("errors.Is(err, ErrDuplicate) = false (err = %v)", err)
		}
	})

	t.Run("unknown adapter", func(t *testing.T) {
		t.Parallel()

		err := &UnknownAdapterError{Adapter: Adapter(99)}
		if got, want := err.Error(), `log: unknown adapter: unknown`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, ErrUnknownAdapter) {
			t.Errorf("errors.Is(err, ErrUnknownAdapter) = false (err = %v)", err)
		}
	})

	t.Run("invalid level", func(t *testing.T) {
		t.Parallel()

		err := &InvalidLevelError{Level: "bogus"}
		if got, want := err.Error(), `log: invalid level: "bogus"`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, ErrInvalidLevel) {
			t.Errorf("errors.Is(err, ErrInvalidLevel) = false (err = %v)", err)
		}
	})

	t.Run("invalid adapter", func(t *testing.T) {
		t.Parallel()

		err := &InvalidAdapterError{Adapter: "bogus"}
		if got, want := err.Error(), `log: invalid adapter: "bogus"`; got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}

		if !errors.Is(err, ErrInvalidAdapter) {
			t.Errorf("errors.Is(err, ErrInvalidAdapter) = false (err = %v)", err)
		}
	})
}
