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
	if !errors.Is(DuplicateAdapterError{Adapter: "jwt"}, ErrDuplicate) {
		t.Fatal("DuplicateAdapterError does not unwrap to ErrDuplicate")
	}
	if !errors.Is(UnknownAdapterError{Adapter: "jwt"}, ErrUnknownAdapter) {
		t.Fatal("UnknownAdapterError does not unwrap to ErrUnknownAdapter")
	}
	if !errors.Is(InvalidAdapterError{Adapter: "x"}, ErrInvalidAdapter) {
		t.Fatal("InvalidAdapterError does not unwrap to ErrInvalidAdapter")
	}
	if !errors.Is(InvalidOptionsError{Reason: "x"}, ErrInvalidOptions) {
		t.Fatal("InvalidOptionsError does not unwrap to ErrInvalidOptions")
	}
}

