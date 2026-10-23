package db

import (
	"context"
	"errors"
	"testing"
)

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
