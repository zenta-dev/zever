package crypto_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/crypto"
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
		{name: "duplicate", err: &crypto.DuplicateAdapterError{Adapter: crypto.AdapterLocal}, want: crypto.ErrDuplicate},
		{name: "unknown", err: &crypto.UnknownAdapterError{Adapter: crypto.AdapterLocal}, want: crypto.ErrUnknownAdapter},
		{name: "invalid_adapter", err: &crypto.InvalidAdapterError{Adapter: "x"}, want: crypto.ErrInvalidAdapter},
		{name: "invalid_options", err: &crypto.InvalidOptionsError{Reason: "x"}, want: crypto.ErrInvalidOptions},
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

func TestErrors_typed_As(t *testing.T) {
	t.Parallel()
	t.Run("invalid_adapter", func(t *testing.T) {
		t.Parallel()
		_, err := crypto.ParseAdapter("bad")
		var iae *crypto.InvalidAdapterError
		if !errors.As(err, &iae) {
			t.Fatalf("err type = %T, want *InvalidAdapterError", err)
		}
		if iae.Adapter != "bad" {
			t.Fatalf("Adapter = %q, want %q", iae.Adapter, "bad")
		}
		if !strings.Contains(iae.Error(), "bad") {
			t.Fatalf("Error() = %q, want adapter name", iae.Error())
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()
		e := &crypto.DuplicateAdapterError{Adapter: crypto.AdapterLocal}
		var de *crypto.DuplicateAdapterError
		if !errors.As(e, &de) {
			t.Fatalf("err type = %T, want *DuplicateAdapterError", e)
		}
		if de.Adapter != crypto.AdapterLocal {
			t.Fatalf("Adapter = %v, want %v", de.Adapter, crypto.AdapterLocal)
		}
		if !strings.Contains(de.Error(), "local") {
			t.Fatalf("Error() = %q, want adapter name", de.Error())
		}
		if !errors.Is(de, crypto.ErrDuplicate) {
			t.Fatalf("Is ErrDuplicate = false")
		}
	})
	t.Run("unknown", func(t *testing.T) {
		t.Parallel()
		e := &crypto.UnknownAdapterError{Adapter: crypto.Adapter(99)}
		var ue *crypto.UnknownAdapterError
		if !errors.As(e, &ue) {
			t.Fatalf("err type = %T, want *UnknownAdapterError", e)
		}
		if ue.Adapter != crypto.Adapter(99) {
			t.Fatalf("Adapter = %v, want 99", ue.Adapter)
		}
		if !strings.Contains(ue.Error(), "unknown") {
			t.Fatalf("Error() = %q, want unknown", ue.Error())
		}
	})
	t.Run("invalid_options", func(t *testing.T) {
		t.Parallel()
		e := &crypto.InvalidOptionsError{Reason: "key is required"}
		var ioe *crypto.InvalidOptionsError
		if !errors.As(e, &ioe) {
			t.Fatalf("err type = %T, want *InvalidOptionsError", e)
		}
		if ioe.Reason != "key is required" {
			t.Fatalf("Reason = %q, want %q", ioe.Reason, "key is required")
		}
		if !strings.Contains(ioe.Error(), "key is required") {
			t.Fatalf("Error() = %q, want reason", ioe.Error())
		}
		if !errors.Is(ioe, crypto.ErrInvalidOptions) {
			t.Fatalf("Is ErrInvalidOptions = false")
		}
	})
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	de := &crypto.DuplicateAdapterError{Adapter: crypto.AdapterLocal}
	if de.Adapter != crypto.AdapterLocal {
		t.Fatalf("DuplicateAdapterError.Adapter = %v", de.Adapter)
	}
	ue := &crypto.UnknownAdapterError{Adapter: crypto.Adapter(5)}
	if ue.Adapter != crypto.Adapter(5) {
		t.Fatalf("UnknownAdapterError.Adapter = %v", ue.Adapter)
	}
	iae := &crypto.InvalidAdapterError{Adapter: "nope"}
	if iae.Adapter != "nope" {
		t.Fatalf("InvalidAdapterError.Adapter = %q", iae.Adapter)
	}
	ioe := &crypto.InvalidOptionsError{Reason: "bad"}
	if ioe.Reason != "bad" {
		t.Fatalf("InvalidOptionsError.Reason = %q", ioe.Reason)
	}
}
