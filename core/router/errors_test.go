package router

import (
	"errors"
	"strings"
	"testing"
)

func TestErrors_sentinels_prefix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
	}{
		{name: "nil_factory", err: ErrNilFactory},
		{name: "duplicate", err: ErrDuplicateAdapter},
		{name: "unknown_adapter", err: ErrUnknownAdapter},
		{name: "invalid_adapter", err: ErrInvalidAdapter},
		{name: "invalid_options", err: ErrInvalidOptions},
		{name: "invalid_method", err: ErrInvalidMethod},
		{name: "malformed_pattern", err: ErrMalformedPattern},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.err == nil {
				t.Fatal("sentinel is nil")
			}
			if !strings.HasPrefix(tc.err.Error(), "router: ") {
				t.Fatalf("sentinel %q missing prefix %q", tc.err.Error(), "router: ")
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
		{name: "duplicate", err: DuplicateAdapterError{Adapter: "fiber"}, want: ErrDuplicateAdapter},
		{name: "unknown", err: UnknownAdapterError{Adapter: "fiber"}, want: ErrUnknownAdapter},
		{name: "invalid_adapter", err: InvalidAdapterError{Adapter: "x"}, want: ErrInvalidAdapter},
		{name: "invalid_options", err: InvalidOptionsError{Reason: "x"}, want: ErrInvalidOptions},
		{name: "invalid_method", err: InvalidMethodError{Method: "x"}, want: ErrInvalidMethod},
		{name: "malformed", err: MalformedPatternError{Pattern: "x", Reason: "y"}, want: ErrMalformedPattern},
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

func TestErrors_duplicate_aliases_compat(t *testing.T) {
	t.Parallel()
	e := DuplicateAdapterError{Adapter: "fiber"}
	if !errors.Is(e, ErrDuplicate) {
		t.Fatal("Is ErrDuplicate alias = false")
	}
	var de DuplicateError
	if !errors.As(e, &de) {
		t.Fatalf("err type = %T, want *DuplicateError alias", e)
	}
	if !errors.Is(ErrDuplicateAdapter, ErrDuplicate) {
		t.Errorf("alias ErrDuplicate does not match ErrDuplicateAdapter")
	}
}

