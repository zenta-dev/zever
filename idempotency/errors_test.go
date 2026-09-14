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
	if !errors.Is(&DuplicateError{Adapter: Memory}, ErrDuplicate) {
		t.Fatal("DuplicateError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(&UnknownAdapterError{Adapter: Memory}, ErrUnknownAdapter) {
		t.Fatal("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(&InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Fatal("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(&InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Fatal("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
	if !errors.Is(&InvalidKeyError{KeyLen: 0}, ErrInvalidKey) {
		t.Fatal("InvalidKeyError does not unwrap to ErrInvalidKey")
	}
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	de := &DuplicateError{Adapter: Redis}
	if de.Adapter != Redis {
		t.Fatalf("DuplicateError.Adapter = %v", de.Adapter)
	}
	ue := &UnknownAdapterError{Adapter: Redis}
	if ue.Adapter != Redis {
		t.Fatalf("UnknownAdapterError.Adapter = %v", ue.Adapter)
	}
	iae := &InvalidAdapterError{Adapter: "nope"}
	if iae.Adapter != "nope" {
		t.Fatalf("InvalidAdapterError.Adapter = %q", iae.Adapter)
	}
	ioe := &InvalidOptionsError{Reason: "bad ttl"}
	if ioe.Reason != "bad ttl" {
		t.Fatalf("InvalidOptionsError.Reason = %q", ioe.Reason)
	}
	if !strings.Contains(ioe.Error(), "bad ttl") {
		t.Fatalf("InvalidOptionsError.Error() = %q, want reason", ioe.Error())
	}
	ike := &InvalidKeyError{KeyLen: 300}
	if ike.KeyLen != 300 {
		t.Fatalf("InvalidKeyError.KeyLen = %d", ike.KeyLen)
	}
	if strings.Contains(ike.Error(), "super-secret") {
		t.Fatal("InvalidKeyError echoes key")
	}
}
