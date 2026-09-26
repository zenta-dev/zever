package db

import (
	"errors"
	"testing"
	"time"
)

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
		check   func(t *testing.T, err error)
	}{
		{
			name:    "happy empty",
			opts:    Options{},
			wantErr: false,
		},
		{
			name: "happy full",
			opts: Options{
				DSN:             "postgres://localhost/db",
				MaxConns:        10,
				MinConns:        2,
				MaxConnLifetime: time.Hour,
				MaxConnIdleTime: time.Minute,
				Path:            "/tmp/test.db",
			},
			wantErr: false,
		},
		{
			name:    "negative max conns",
			opts:    Options{MaxConns: -1},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, ErrInvalidOptions) {
					t.Errorf("does not unwrap to ErrInvalidOptions: %v", err)
				}
			},
		},
		{
			name:    "negative min conns",
			opts:    Options{MinConns: -5},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, ErrInvalidOptions) {
					t.Errorf("does not unwrap to ErrInvalidOptions: %v", err)
				}
			},
		},
		{
			name:    "negative lifetime",
			opts:    Options{MaxConnLifetime: -time.Second},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, ErrInvalidOptions) {
					t.Errorf("does not unwrap to ErrInvalidOptions: %v", err)
				}
			},
		},
		{
			name:    "negative idle time",
			opts:    Options{MaxConnIdleTime: -time.Second},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()
				if !errors.Is(err, ErrInvalidOptions) {
					t.Errorf("does not unwrap to ErrInvalidOptions: %v", err)
				}
			},
		},
		{
			name: "joined multiples",
			opts: Options{
				MaxConns:        -1,
				MinConns:        -2,
				MaxConnLifetime: -time.Second,
				MaxConnIdleTime: -time.Minute,
			},
			wantErr: true,
			check: func(t *testing.T, err error) {
				t.Helper()

				var invErr *InvalidOptionsError
				if !errors.As(err, &invErr) {
					t.Fatalf("joined error has no *InvalidOptionsError: %v", err)
				}

				if !errors.Is(err, ErrInvalidOptions) {
					t.Errorf("does not unwrap to ErrInvalidOptions: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.opts.Validate()
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if tt.check != nil {
				tt.check(t, err)
			}
		})
	}
}
