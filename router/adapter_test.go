package router

import (
	"errors"
	"strings"
	"testing"
)

func TestAdapter_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a    Adapter
		want string
	}{
		{name: "fiber", a: AdapterFiber, want: "fiber"},
		{name: "stdhttp", a: AdapterStdHTTP, want: "stdhttp"},
		{name: "unknown", a: Adapter(99), want: "unknown"},
		{name: "negative", a: Adapter(-1), want: "unknown"},
		{name: "zero is fiber", a: Adapter(0), want: "fiber"},
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
		want Adapter
		ok   bool
	}{
		{name: "fiber", in: "fiber", want: AdapterFiber, ok: true},
		{name: "stdhttp", in: "stdhttp", want: AdapterStdHTTP, ok: true},
		{name: "empty", in: "", want: AdapterFiber, ok: false},
		{name: "uppercase fiber", in: "Fiber", want: AdapterFiber, ok: false},
		{name: "upper stdhttp", in: "STDHTTP", want: AdapterFiber, ok: false},
		{name: "padded", in: " fiber", want: AdapterFiber, ok: false},
		{name: "trailing space", in: "fiber ", want: AdapterFiber, ok: false},
		{name: "unknown", in: "chi", want: AdapterFiber, ok: false},
		{name: "zero string", in: "0", want: AdapterFiber, ok: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseAdapter(tc.in)
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
				var iae *InvalidAdapterError
				if !errors.As(err, &iae) {
					t.Fatalf("err type = %T, want *InvalidAdapterError", err)
				}
				if iae.Adapter != tc.in {
					t.Fatalf("Adapter = %q, want %q", iae.Adapter, tc.in)
				}
				if !errors.Is(err, ErrInvalidAdapter) {
					t.Fatalf("err = %v, want ErrInvalidAdapter", err)
				}
				if got != AdapterFiber {
					t.Fatalf("ParseAdapter(%q) adapter = %v, want AdapterFiber", tc.in, got)
				}
				if tc.in != "" && !strings.Contains(err.Error(), tc.in) {
					t.Fatalf("error should contain adapter name, got %q", err.Error())
				}
			}
		})
	}
}

func TestParseAdapter_String_consistency(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"fiber", "stdhttp"} {
		in := in
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			a, err := ParseAdapter(in)
			if err != nil {
				t.Fatalf("ParseAdapter(%q) err = %v", in, err)
			}
			if a.String() != in {
				t.Fatalf("String() = %q, want %q", a.String(), in)
			}
		})
	}
}

func TestAdapter_Parse_unknown_carries_adapter(t *testing.T) {
	t.Parallel()
	_, err := ParseAdapter("bad-adapter")
	if err == nil {
		t.Fatal("expected error")
	}
	var iae *InvalidAdapterError
	if !errors.As(err, &iae) {
		t.Fatalf("type = %T", err)
	}
	if !errors.Is(err, ErrInvalidAdapter) {
		t.Fatal("Is ErrInvalidAdapter false")
	}
	if iae.Adapter != "bad-adapter" {
		t.Fatalf("Adapter = %q", iae.Adapter)
	}
}
