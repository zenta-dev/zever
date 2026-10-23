package idempotency

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinels_match(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"in_progress", ErrInProgress, "idempotency: "},
		{"key_mismatch", ErrKeyMismatch, "idempotency: "},
		{"closed", ErrClosed, "idempotency: "},
		{"invalid_key", ErrInvalidKey, "idempotency: "},
		{"nil_factory", ErrNilFactory, "idempotency: "},
		{"duplicate", ErrDuplicate, "idempotency: "},
		{"unknown_adapter", ErrUnknownAdapter, "idempotency: "},
		{"invalid_adapter", ErrInvalidAdapter, "idempotency: "},
		{"invalid_options", ErrInvalidOptions, "idempotency: "},
		{"fingerprint_too_large", ErrFingerprintTooLarge, "idempotency: "},
		{"corrupt_record", ErrCorruptRecord, "idempotency: "},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil {
				t.Fatal("sentinel is nil")
			}
			if !strings.HasPrefix(tc.err.Error(), tc.want) {
				t.Fatalf("sentinel %q missing prefix %q", tc.err.Error(), tc.want)
			}
		})
	}
}

func TestErrors_unwrap_carries_sentinel(t *testing.T) {
	t.Parallel()
	if !errors.Is(DuplicateAdapterError{Adapter: "memory"}, ErrDuplicate) {
		t.Fatal("DuplicateAdapterError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(UnknownAdapterError{Adapter: "memory"}, ErrUnknownAdapter) {
		t.Fatal("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Fatal("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Fatal("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(InvalidKeyError{KeyLen: 0}, ErrInvalidKey) {
		t.Fatal("InvalidKeyError does not unwrap to ErrInvalidKey")
	}
}

