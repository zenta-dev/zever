package httpclient

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

// TestReadLimited_boundaries covers empty bodies, exact limits, and clamping.
func TestReadLimited_boundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    []byte
		limit   int64
		wantLen int
		wantErr error
	}{
		{name: "empty body", data: nil, limit: 0, wantLen: 0},
		{name: "exact limit", data: []byte("abcde"), limit: 5, wantLen: 5},
		{name: "one over limit", data: []byte("abcdef"), limit: 5, wantErr: ErrTooLarge},
		{name: "negative limit clamps to zero", data: []byte{}, limit: -1, wantLen: 0},
		{name: "negative limit nonempty", data: []byte("a"), limit: -1, wantErr: ErrTooLarge},
		{name: "zero limit empty", data: []byte{}, limit: 0, wantLen: 0},
		{name: "zero limit nonempty", data: []byte("a"), limit: 0, wantErr: ErrTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ReadLimited(t.Context(), bytes.NewReader(tt.data), tt.limit)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ReadLimited() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ReadLimited() error = %v, want nil", err)
			}

			if len(got) != tt.wantLen {
				t.Fatalf("ReadLimited() len = %d, want %d", len(got), tt.wantLen)
			}
		})
	}
}

// TestReadLimited_cancelledContext verifies a cancelled context aborts the read.
func TestReadLimited_cancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := ReadLimited(ctx, bytes.NewReader([]byte("data")), 16)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ReadLimited(cancelled) error = %v, want context.Canceled", err)
	}
}
