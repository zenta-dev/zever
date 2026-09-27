package crypto_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

func TestAdapter_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a    crypto.Adapter
		want string
	}{
		{name: "local", a: crypto.AdapterLocal, want: "local"},
		{name: "unknown", a: crypto.Adapter(""), want: "unknown"},
		{name: "negative", a: crypto.Adapter(""), want: "unknown"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.a.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseAdapter_roundtrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want crypto.Adapter
		ok   bool
	}{
		{name: "local", in: "local", want: crypto.AdapterLocal, ok: true},
		{name: "empty", in: "", want: crypto.Adapter(""), ok: false},
		{name: "uppercase", in: "Local", want: crypto.Adapter("Local"), ok: true},
		{name: "LOCAL", in: "LOCAL", want: crypto.Adapter("LOCAL"), ok: true},
		{name: "unknown", in: "redis", want: crypto.Adapter("redis"), ok: true},
		{name: "vault", in: "vault", want: crypto.Adapter("vault"), ok: true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := crypto.ParseAdapter(tc.in)
			if tc.ok {
				if err != nil {
					t.Fatalf("ParseAdapter(%q) err = %v, want nil", tc.in, err)
				}
				if got != tc.want {
					t.Fatalf("ParseAdapter(%q) = %v, want %v", tc.in, got, tc.want)
				}
				if got.String() != tc.in {
					t.Fatalf("roundtrip String() = %q, want %q", got.String(), tc.in)
				}
			} else {
				if err == nil {
					t.Fatalf("ParseAdapter(%q) expected error, got nil", tc.in)
				}
				var iae *crypto.InvalidAdapterError
				if !errors.As(err, &iae) {
					t.Fatalf("err type = %T, want *InvalidAdapterError", err)
				}
				if iae.Adapter != tc.in {
					t.Fatalf("Adapter = %q, want %q", iae.Adapter, tc.in)
				}
				if !errors.Is(err, crypto.ErrInvalidAdapter) {
					t.Fatalf("err = %v, want ErrInvalidAdapter", err)
				}
				if got != crypto.Adapter("") {
					t.Fatalf("ParseAdapter(%q) adapter = %v, want AdapterLocal", tc.in, got)
				}
				if !strings.Contains(err.Error(), tc.in) && tc.in != "" {
					t.Fatalf("error should contain adapter name, got %q", err.Error())
				}
			}
		})
	}
}

func TestParseAdapter_String_consistency(t *testing.T) {
	t.Parallel()
	a, err := crypto.ParseAdapter("local")
	if err != nil {
		t.Fatalf("ParseAdapter(local) err = %v", err)
	}
	if a.String() != "local" {
		t.Fatalf("String() = %q, want %q", a.String(), "local")
	}
}

func TestAdapter_Parse_unknown_carries_adapter(t *testing.T) {
	t.Parallel()
	got, err := crypto.ParseAdapter("bad-adapter")
	if err != nil {
		t.Fatalf("ParseAdapter(bad-adapter) err = %v, want nil (open adapter)", err)
	}
	if got != crypto.Adapter("bad-adapter") {
		t.Fatalf("ParseAdapter(bad-adapter) = %v, want %v", got, crypto.Adapter("bad-adapter"))
	}
}
