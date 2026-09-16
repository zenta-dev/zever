package db

import (
	"context"
	"errors"
	"testing"
)

type stubRows struct{}

func (r *stubRows) Next() bool                 { return false }
func (r *stubRows) Scan(_ ...any) error        { return nil }
func (r *stubRows) Close() error               { return nil }
func (r *stubRows) Columns() ([]string, error) { return []string{"id"}, nil }
func (r *stubRows) Err() error                 { return nil }

type stubDB struct {
	dialect string
}

func (s *stubDB) Query(_ context.Context, _ string, _ ...any) (Rows, error) {
	return &stubRows{}, nil
}

func (s *stubDB) Exec(_ context.Context, _ string, _ ...any) (int64, error) {
	return 1, nil
}

func (s *stubDB) Ping(_ context.Context) error { return nil }

func (s *stubDB) Close(_ context.Context) error { return nil }

func (s *stubDB) Dialect() string { return s.dialect }

func TestRegister(t *testing.T) {
	t.Parallel()

	t.Run("nil factory", func(t *testing.T) {
		t.Parallel()

		err := Register(Adapter(1001), nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !errors.Is(err, ErrNilFactory) {
			t.Errorf("does not unwrap to ErrNilFactory: %v", err)
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		adapter := Adapter(1002)
		factory := func(Options) (DB, error) { return &stubDB{dialect: "sqlite"}, nil }

		if err := Register(adapter, factory); err != nil {
			t.Fatalf("first register failed: %v", err)
		}

		err := Register(adapter, factory)
		if err == nil {
			t.Fatal("expected duplicate error, got nil")
		}

		var dupErr *DuplicateAdapterError
		if !errors.As(err, &dupErr) {
			t.Fatalf("error is %T, want *DuplicateAdapterError", err)
		}

		if !errors.Is(err, ErrDuplicateAdapter) {
			t.Errorf("does not unwrap to ErrDuplicateAdapter: %v", err)
		}

		if dupErr.Adapter != adapter {
			t.Errorf("Adapter = %v, want %v", dupErr.Adapter, adapter)
		}
	})

	t.Run("happy then open dialect", func(t *testing.T) {
		t.Parallel()

		adapter := Adapter(1003)
		factory := func(Options) (DB, error) { return &stubDB{dialect: "custom"}, nil }

		if err := Register(adapter, factory); err != nil {
			t.Fatalf("register failed: %v", err)
		}

		db, err := Open(adapter, Options{})
		if err != nil {
			t.Fatalf("open failed: %v", err)
		}

		if db.Dialect() != "custom" {
			t.Errorf("Dialect() = %q, want custom", db.Dialect())
		}
	})
}

func TestOpen(t *testing.T) {
	t.Parallel()

	t.Run("unknown adapter", func(t *testing.T) {
		t.Parallel()

		_, err := Open(Adapter(1999), Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var unkErr *UnknownAdapterError
		if !errors.As(err, &unkErr) {
			t.Fatalf("error is %T, want *UnknownAdapterError", err)
		}

		if !errors.Is(err, ErrUnknownAdapter) {
			t.Errorf("does not unwrap to ErrUnknownAdapter: %v", err)
		}
	})

	t.Run("validate fail closed", func(t *testing.T) {
		t.Parallel()

		// Even for an unregistered adapter, invalid options must win:
		// Validate runs before registry lookup.
		_, err := Open(Adapter(2999), Options{MaxConns: -1})
		if err == nil {
			t.Fatal("expected validation error, got nil")
		}

		if !errors.Is(err, ErrInvalidOptions) {
			t.Errorf("expected ErrInvalidOptions, got %v", err)
		}

		var unkErr *UnknownAdapterError
		if errors.As(err, &unkErr) {
			t.Error("validation failure leaked into unknown-adapter error")
		}
	})

	t.Run("factory error wrapped", func(t *testing.T) {
		t.Parallel()

		adapter := Adapter(1004)
		sentinel := errors.New("backend down")

		if err := Register(adapter, func(Options) (DB, error) { return nil, sentinel }); err != nil {
			t.Fatalf("register failed: %v", err)
		}

		_, err := Open(adapter, Options{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		if !errors.Is(err, sentinel) {
			t.Errorf("does not wrap factory error: %v", err)
		}
	})

	t.Run("table validate failures", func(t *testing.T) {
		t.Parallel()

		adapter := Adapter(1005)
		if err := Register(adapter, func(Options) (DB, error) { return &stubDB{}, nil }); err != nil {
			t.Fatalf("register failed: %v", err)
		}

		tests := []struct {
			name string
			opts Options
		}{
			{name: "max conns", opts: Options{MaxConns: -1}},
			{name: "min conns", opts: Options{MinConns: -1}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				if _, err := Open(adapter, tt.opts); !errors.Is(err, ErrInvalidOptions) {
					t.Errorf("expected ErrInvalidOptions, got %v", err)
				}
			})
		}
	})
}
