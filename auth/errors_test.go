package auth

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
		{"token_expired", ErrTokenExpired, "auth: "},
		{"token_revoked", ErrTokenRevoked, "auth: "},
		{"invalid_token", ErrInvalidToken, "auth: "},
		{"not_supported", ErrNotSupported, "auth: "},
		{"nil_factory", ErrNilFactory, "auth: "},
		{"duplicate", ErrDuplicate, "auth: "},
		{"unknown_adapter", ErrUnknownAdapter, "auth: "},
		{"invalid_adapter", ErrInvalidAdapter, "auth: "},
		{"invalid_options", ErrInvalidOptions, "auth: "},
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
	if !errors.Is(&DuplicateError{Adapter: JWT}, ErrDuplicate) {
		t.Fatal("DuplicateError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(&UnknownAdapterError{Adapter: JWT}, ErrUnknownAdapter) {
		t.Fatal("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(&InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Fatal("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(&InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Fatal("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
}

func TestErrors_carried_fields(t *testing.T) {
	t.Parallel()
	de := &DuplicateError{Adapter: Session}
	if de.Adapter != Session {
		t.Fatalf("DuplicateError.Adapter = %v", de.Adapter)
	}
	ue := &UnknownAdapterError{Adapter: OIDC}
	if ue.Adapter != OIDC {
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
}
