package db

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantSub  string
		sentinel error
	}{
		{
			name:     "invalid adapter",
			err:      &InvalidAdapterError{Adapter: "mysql"},
			wantSub:  "db: invalid adapter",
			sentinel: ErrInvalidAdapter,
		},
		{
			name:     "duplicate adapter",
			err:      &DuplicateAdapterError{Adapter: SQLite},
			wantSub:  "db: duplicate adapter",
			sentinel: ErrDuplicateAdapter,
		},
		{
			name:     "unknown adapter",
			err:      &UnknownAdapterError{Adapter: Adapter("test-42")},
			wantSub:  "db: unknown adapter",
			sentinel: ErrUnknownAdapter,
		},
		{
			name:     "invalid options",
			err:      &InvalidOptionsError{Reason: "max conns must not be negative"},
			wantSub:  "db: invalid options",
			sentinel: ErrInvalidOptions,
		},
		{
			name:     "tx begin",
			err:      &TxError{Op: "begin", Err: errors.New("boom")},
			wantSub:  "db: transaction begin",
			sentinel: nil,
		},
		{
			name:     "tx commit",
			err:      &TxError{Op: "commit", Err: errors.New("boom")},
			wantSub:  "db: transaction commit",
			sentinel: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.err.Error(); !strings.Contains(got, tt.wantSub) {
				t.Errorf("Error() = %q, want substring %q", got, tt.wantSub)
			}

			if tt.sentinel != nil && !errors.Is(tt.err, tt.sentinel) {
				t.Errorf("errors.Is(%v, sentinel) = false", tt.err)
			}
		})
	}
}

func TestDuplicateAliases_compat(t *testing.T) {
	t.Parallel()

	err := &DuplicateAdapterError{Adapter: SQLite}
	if !errors.Is(err, ErrDuplicate) {
		t.Error("does not unwrap to ErrDuplicate alias")
	}

	var target *DuplicateError
	if !errors.As(err, &target) {
		t.Error("errors.As failed for DuplicateError alias")
	}

	if !errors.Is(ErrDuplicateAdapter, ErrDuplicate) {
		t.Errorf("alias ErrDuplicate does not match ErrDuplicateAdapter")
	}
}

func TestErrorUnwrap(t *testing.T) {
	t.Parallel()

	t.Run("invalid adapter unwraps", func(t *testing.T) {
		t.Parallel()

		err := &InvalidAdapterError{Adapter: "x"}
		if !errors.Is(err, ErrInvalidAdapter) {
			t.Error("does not unwrap to ErrInvalidAdapter")
		}

		if !errors.Is(err.Unwrap(), ErrInvalidAdapter) {
			t.Errorf("Unwrap() does not match ErrInvalidAdapter")
		}
	})

	t.Run("duplicate unwraps", func(t *testing.T) {
		t.Parallel()

		err := &DuplicateAdapterError{Adapter: Postgres}
		if !errors.Is(err, ErrDuplicateAdapter) {
			t.Error("does not unwrap to ErrDuplicateAdapter")
		}

		if !errors.Is(err.Unwrap(), ErrDuplicateAdapter) {
			t.Errorf("Unwrap() does not match ErrDuplicateAdapter")
		}
	})

	t.Run("unknown unwraps", func(t *testing.T) {
		t.Parallel()

		err := &UnknownAdapterError{Adapter: Postgres}
		if !errors.Is(err, ErrUnknownAdapter) {
			t.Error("does not unwrap to ErrUnknownAdapter")
		}

		if !errors.Is(err.Unwrap(), ErrUnknownAdapter) {
			t.Errorf("Unwrap() does not match ErrUnknownAdapter")
		}
	})

	t.Run("invalid options unwraps", func(t *testing.T) {
		t.Parallel()

		err := &InvalidOptionsError{Reason: "r"}
		if !errors.Is(err, ErrInvalidOptions) {
			t.Error("does not unwrap to ErrInvalidOptions")
		}

		if !errors.Is(err.Unwrap(), ErrInvalidOptions) {
			t.Errorf("Unwrap() does not match ErrInvalidOptions")
		}
	})

	t.Run("tx unwraps inner", func(t *testing.T) {
		t.Parallel()

		inner := errors.New("inner")
		err := &TxError{Op: "commit", Err: inner}

		if !errors.Is(err.Unwrap(), inner) {
			t.Error("Unwrap() did not return inner error")
		}

		if !errors.Is(err, inner) {
			t.Error("errors.Is does not find inner error")
		}
	})

	t.Run("sentinels distinct", func(t *testing.T) {
		t.Parallel()

		sentinels := []error{
			ErrNilFactory,
			ErrDuplicateAdapter,
			ErrUnknownAdapter,
			ErrInvalidAdapter,
			ErrInvalidOptions,
			ErrNotFound,
			ErrNestedTx,
			ErrTxUnsupported,
		}

		for _, s := range sentinels {
			if s == nil {
				t.Error("sentinel is nil")
			}

			if !strings.HasPrefix(s.Error(), "db: ") {
				t.Errorf("sentinel %q missing db prefix", s.Error())
			}
		}

		if errors.Is(ErrNotFound, ErrInvalidOptions) {
			t.Error("ErrNotFound should not match ErrInvalidOptions")
		}
	})
}
