package db

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestDBEdge_OpenConcurrent(t *testing.T) {
	t.Parallel()

	adapter := dbFreshAdapter()
	if err := Register(adapter, func(Options) (DB, error) { return &stubDB{dialect: "sqlite"}, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := Open(adapter, Options{}); err != nil {
				t.Errorf("Open err = %v", err)
			}
		}()
	}

	wg.Wait()
}

func TestDBEdge_RegisterConcurrentUnique(t *testing.T) {
	t.Parallel()

	var wg sync.WaitGroup

	errs := make([]error, 20)

	for i := range errs {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			errs[i] = Register(dbFreshAdapter(), func(Options) (DB, error) { return &stubDB{}, nil })
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Register worker %d err = %v", i, err)
		}
	}
}

func TestDBEdge_WithTxNilDB(t *testing.T) {
	t.Parallel()

	err := WithTx(t.Context(), nil, nil, func(context.Context, Tx) error {
		t.Error("fn must not run for a nil DB")

		return nil
	})
	if !errors.Is(err, ErrTxUnsupported) {
		t.Fatalf("WithTx(nil) err = %v, want ErrTxUnsupported", err)
	}
}

func TestDBEdge_OptionsValidateBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{name: "zero valid", opts: Options{}},
		{name: "one valid", opts: Options{MaxConns: 1, MinConns: 1}},
		{name: "negative max invalid", opts: Options{MaxConns: -1}, wantErr: true},
		{name: "negative idle invalid", opts: Options{MaxConnIdleTime: -1}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.opts.Validate() != nil; got != tt.wantErr {
				t.Fatalf("Validate() error present = %v, want %v", got, tt.wantErr)
			}
		})
	}
}
