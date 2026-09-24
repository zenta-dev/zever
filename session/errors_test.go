package session

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
		{"not_found", ErrNotFound, "session: "},
		{"closed", ErrClosed, "session: "},
		{"nil_factory", ErrNilFactory, "session: "},
		{"duplicate", ErrDuplicate, "session: "},
		{"unknown_adapter", ErrUnknownAdapter, "session: "},
		{"invalid_adapter", ErrInvalidAdapter, "session: "},
		{"invalid_options", ErrInvalidOptions, "session: "},
		{"invalid_id", ErrInvalidID, "session: "},
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
	if !errors.Is(&DuplicateAdapterError{Adapter: Memory}, ErrDuplicate) {
		t.Fatal("DuplicateAdapterError does not unwrap to ErrDuplicate")
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
	if !errors.Is(&InvalidIDError{IDLen: 0}, ErrInvalidID) {
		t.Fatal("InvalidIDError does not unwrap to ErrInvalidID")
	}
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	de := &DuplicateAdapterError{Adapter: Memory}
	if de.Adapter != Memory {
		t.Fatalf("DuplicateAdapterError.Adapter = %v", de.Adapter)
	}
	ue := &UnknownAdapterError{Adapter: Memory}
	if ue.Adapter != Memory {
		t.Fatalf("UnknownAdapterError.Adapter = %v", ue.Adapter)
	}
	iae := &InvalidAdapterError{Adapter: "nope"}
	if iae.Adapter != "nope" {
		t.Fatalf("InvalidAdapterError.Adapter = %q", iae.Adapter)
	}
	if !strings.Contains(iae.Error(), "nope") {
		t.Fatalf("InvalidAdapterError.Error() = %q, want adapter name", iae.Error())
	}
	ioe := &InvalidOptionsError{Reason: "bad ttl"}
	if ioe.Reason != "bad ttl" {
		t.Fatalf("InvalidOptionsError.Reason = %q", ioe.Reason)
	}
	if !strings.Contains(ioe.Error(), "bad ttl") {
		t.Fatalf("InvalidOptionsError.Error() = %q, want reason", ioe.Error())
	}
	iie := &InvalidIDError{IDLen: 36}
	if iie.IDLen != 36 {
		t.Fatalf("InvalidIDError.IDLen = %d", iie.IDLen)
	}
	if strings.Contains(iie.Error(), "super-secret") {
		t.Fatal("InvalidIDError echoes ID")
	}
}
