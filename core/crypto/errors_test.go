package crypto_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

func TestErrors_sentinels_prefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
	}{
		{name: "invalid_key", err: crypto.ErrInvalidKey},
		{name: "key_not_found", err: crypto.ErrKeyNotFound},
		{name: "not_supported", err: crypto.ErrNotSupported},
		{name: "integrity", err: crypto.ErrIntegrity},
		{name: "nil_factory", err: crypto.ErrNilFactory},
		{name: "duplicate", err: crypto.ErrDuplicate},
		{name: "unknown_adapter", err: crypto.ErrUnknownAdapter},
		{name: "invalid_adapter", err: crypto.ErrInvalidAdapter},
		{name: "invalid_options", err: crypto.ErrInvalidOptions},
		{name: "dev_key", err: crypto.ErrDevKey},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil {
				t.Fatal("sentinel is nil")
			}
			if !strings.HasPrefix(tc.err.Error(), "crypto: ") {
				t.Fatalf("sentinel %q missing prefix %q", tc.err.Error(), "crypto: ")
			}
		})
	}
}

func TestErrors_unwrap_carries_sentinel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want error
	}{
		{name: "duplicate", err: crypto.DuplicateAdapterError{Adapter: "local"}, want: crypto.ErrDuplicate},
		{name: "unknown", err: crypto.UnknownAdapterError{Adapter: "local"}, want: crypto.ErrUnknownAdapter},
		{name: "invalid_adapter", err: crypto.InvalidAdapterError{Adapter: "x"}, want: crypto.ErrInvalidAdapter},
		{name: "invalid_options", err: crypto.InvalidOptionsError{Reason: "x"}, want: crypto.ErrInvalidOptions},
		{name: "dev_key", err: crypto.DevKeyError{}, want: crypto.ErrDevKey},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if !errors.Is(tc.err, tc.want) {
				t.Fatalf("errors.Is(%T, %v) = false, want true", tc.err, tc.want)
			}
		})
	}
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	de := crypto.DuplicateAdapterError{Adapter: "local"}
	if de.Adapter != "local" {
		t.Fatalf("DuplicateAdapterError.Adapter = %v", de.Adapter)
	}
	ue := crypto.UnknownAdapterError{Adapter: "test-5"}
	if ue.Adapter != "test-5" {
		t.Fatalf("UnknownAdapterError.Adapter = %v", ue.Adapter)
	}
	iae := crypto.InvalidAdapterError{Adapter: "nope"}
	if iae.Adapter != "nope" {
		t.Fatalf("InvalidAdapterError.Adapter = %q", iae.Adapter)
	}
	ioe := crypto.InvalidOptionsError{Reason: "bad"}
	if ioe.Reason != "bad" {
		t.Fatalf("InvalidOptionsError.Reason = %q", ioe.Reason)
	}
}
