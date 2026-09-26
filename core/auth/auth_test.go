package auth

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var testAdapterSeq int32 = 1000

func freshAdapter() Adapter {
	return Adapter(fmt.Sprintf("test-%d", atomic.AddInt32(&testAdapterSeq, 1)))
}

func TestRegister_nil_factory_fails(t *testing.T) {
	a := freshAdapter()
	err := Register(a, nil)
	if err == nil {
		t.Fatal("Register nil factory expected error, got nil")
	}
	if !errors.Is(err, ErrNilFactory) {
		t.Fatalf("Register nil err = %v, want ErrNilFactory", err)
	}
}

func TestRegister_duplicate_fails(t *testing.T) {
	a := freshAdapter()
	factory := func(Options) (Auth, error) { return &stubAuth{}, nil }
	if err := Register(a, factory); err != nil {
		t.Fatalf("first Register err = %v", err)
	}
	err := Register(a, factory)
	if err == nil {
		t.Fatal("duplicate Register expected error, got nil")
	}
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate Register err = %v, want ErrDuplicate", err)
	}
	var de *DuplicateError
	if !errors.As(err, &de) {
		t.Fatalf("duplicate err type = %T, want *DuplicateError", err)
	}
	if de.Adapter != a {
		t.Fatalf("DuplicateError.Adapter = %v, want %v", de.Adapter, a)
	}
}

func TestOpen_unknown_adapter_fails(t *testing.T) {
	a := freshAdapter()
	_, err := Open(a, Options{})
	if err == nil {
		t.Fatal("Open unknown expected error, got nil")
	}
	if !errors.Is(err, ErrUnknownAdapter) {
		t.Fatalf("Open unknown err = %v, want ErrUnknownAdapter", err)
	}
	var ue *UnknownAdapterError
	if !errors.As(err, &ue) {
		t.Fatalf("Open unknown err type = %T, want *UnknownAdapterError", err)
	}
	if ue.Adapter != a {
		t.Fatalf("UnknownAdapterError.Adapter = %v, want %v", ue.Adapter, a)
	}
}

func TestOpen_factory_error_wrapped(t *testing.T) {
	a := freshAdapter()
	sentinel := errors.New("boom")
	factory := func(Options) (Auth, error) { return nil, sentinel }
	if err := Register(a, factory); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	_, err := Open(a, Options{})
	if err == nil {
		t.Fatal("Open factory error expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Open err = %v, want wrapped sentinel", err)
	}
	if !strings.HasPrefix(err.Error(), "auth: open ") {
		t.Fatalf("Open err = %q, want %q prefix", err.Error(), "auth: open ")
	}
}

type stubAuth struct{}

func (stubAuth) Issue(context.Context, string, map[string]any, time.Duration) (Token, error) {
	return Token{}, nil
}

func (stubAuth) Verify(context.Context, string) (Claims, error) {
	return Claims{}, nil
}

func (stubAuth) Revoke(context.Context, string) error { return nil }

func (stubAuth) Close() error { return nil }

func TestOpen_registered_success(t *testing.T) {
	a := freshAdapter()
	want := stubAuth{}
	if err := Register(a, func(Options) (Auth, error) { return want, nil }); err != nil {
		t.Fatalf("Register err = %v", err)
	}
	got, err := Open(a, Options{})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if _, ok := got.(stubAuth); !ok {
		t.Fatalf("Open returned %T, want stubAuth", got)
	}
}

func TestClaims_Clone_independent(t *testing.T) {
	t.Parallel()
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	orig := Claims{
		Subject:   "user-1",
		ExpiresAt: exp,
		Custom: map[string]any{
			"role": "admin",
			"nested": map[string]any{
				"key": "val",
			},
			"list": []any{"a", map[string]any{"deep": "x"}},
			"bin":  []byte{1, 2, 3},
			"tags": []string{"a", "b"},
			"strm": map[string]string{"k": "v"},
		},
	}
	cp := orig.Clone()
	if !reflect.DeepEqual(cp, orig) {
		t.Fatalf("Clone() = %#v, want %#v", cp, orig)
	}
	// Mutate every nested shape in the copy; original must not move.
	cp.Custom["role"] = "mutant"
	mustStrMap(t, cp.Custom["nested"])["key"] = "mutant"
	mustAnySlice(t, cp.Custom["list"])[0] = "mutant"
	mustStrMap(t, mustAnySlice(t, cp.Custom["list"])[1])["deep"] = "mutant"
	mustBytes(t, cp.Custom["bin"])[0] = 9
	mustStrSlice(t, cp.Custom["tags"])[0] = "mutant"
	mustStrStrMap(t, cp.Custom["strm"])["k"] = "mutant"
	if orig.Custom["role"] != "admin" {
		t.Fatalf("Clone shares top-level value: role = %v", orig.Custom["role"])
	}
	if mustStrMap(t, orig.Custom["nested"])["key"] != "val" {
		t.Fatalf("Clone shares nested map: %v", orig.Custom["nested"])
	}
	if mustAnySlice(t, orig.Custom["list"])[0] != "a" {
		t.Fatalf("Clone shares slice element: %v", orig.Custom["list"])
	}
	if mustStrMap(t, mustAnySlice(t, orig.Custom["list"])[1])["deep"] != "x" {
		t.Fatalf("Clone shares deep map in slice: %v", orig.Custom["list"])
	}
	if mustBytes(t, orig.Custom["bin"])[0] != 1 {
		t.Fatalf("Clone shares bytes: %v", orig.Custom["bin"])
	}
	if mustStrSlice(t, orig.Custom["tags"])[0] != "a" {
		t.Fatalf("Clone shares []string: %v", orig.Custom["tags"])
	}
	if mustStrStrMap(t, orig.Custom["strm"])["k"] != "v" {
		t.Fatalf("Clone shares map[string]string: %v", orig.Custom["strm"])
	}
	if cp.Subject != "user-1" || !cp.ExpiresAt.Equal(exp) {
		t.Fatalf("Clone dropped scalar fields: %#v", cp)
	}
}

func mustStrMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("value %T is not map[string]any", v)
	}
	return m
}

func mustAnySlice(t *testing.T, v any) []any {
	t.Helper()
	s, ok := v.([]any)
	if !ok {
		t.Fatalf("value %T is not []any", v)
	}
	return s
}

func mustBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, ok := v.([]byte)
	if !ok {
		t.Fatalf("value %T is not []byte", v)
	}
	return b
}

func mustStrSlice(t *testing.T, v any) []string {
	t.Helper()
	s, ok := v.([]string)
	if !ok {
		t.Fatalf("value %T is not []string", v)
	}
	return s
}

func mustStrStrMap(t *testing.T, v any) map[string]string {
	t.Helper()
	m, ok := v.(map[string]string)
	if !ok {
		t.Fatalf("value %T is not map[string]string", v)
	}
	return m
}

func TestClaims_Clone_nil_custom(t *testing.T) {
	t.Parallel()
	cp := Claims{Subject: "s"}.Clone()
	if cp.Custom != nil {
		t.Fatalf("Clone nil Custom = %v, want nil", cp.Custom)
	}
	if cp.Subject != "s" {
		t.Fatalf("Clone Subject = %q, want %q", cp.Subject, "s")
	}
}

func TestCloneValue_table(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		in    any
		check func(t *testing.T, in, out any)
	}{
		{
			name: "nil",
			in:   nil,
			check: func(t *testing.T, _, out any) {
				t.Helper()
				if out != nil {
					t.Fatalf("CloneValue(nil) = %#v, want nil", out)
				}
			},
		},
		{
			name: "scalar_passthrough",
			in:   "hello",
			check: func(t *testing.T, _, out any) {
				t.Helper()
				if out != "hello" {
					t.Fatalf("CloneValue = %#v, want passthrough", out)
				}
			},
		},
		{
			name: "map_deep",
			in:   map[string]any{"a": map[string]any{"b": "c"}},
			check: func(t *testing.T, in, out any) {
				t.Helper()
				got := mustStrMap(t, out)
				mustStrMap(t, got["a"])["b"] = "mutant"
				inMap := mustStrMap(t, in)
				if mustStrMap(t, inMap["a"])["b"] != "c" {
					t.Fatal("CloneValue shares nested map")
				}
			},
		},
		{
			name: "slice_any_deep",
			in:   []any{map[string]any{"k": "v"}},
			check: func(t *testing.T, in, out any) {
				t.Helper()
				got := mustAnySlice(t, out)
				mustStrMap(t, got[0])["k"] = "mutant"
				inSlice := mustAnySlice(t, in)
				if mustStrMap(t, inSlice[0])["k"] != "v" {
					t.Fatal("CloneValue shares []any element")
				}
			},
		},
		{
			name: "bytes_copy",
			in:   []byte{1, 2},
			check: func(t *testing.T, in, out any) {
				t.Helper()
				got := mustBytes(t, out)
				got[0] = 9
				if mustBytes(t, in)[0] != 1 {
					t.Fatal("CloneValue shares []byte")
				}
			},
		},
		{
			name: "string_map_copy",
			in:   map[string]string{"k": "v"},
			check: func(t *testing.T, in, out any) {
				t.Helper()
				got := mustStrStrMap(t, out)
				got["k"] = "mutant"
				if mustStrStrMap(t, in)["k"] != "v" {
					t.Fatal("CloneValue shares map[string]string")
				}
			},
		},
		{
			name: "string_slice_copy",
			in:   []string{"a"},
			check: func(t *testing.T, in, out any) {
				t.Helper()
				got := mustStrSlice(t, out)
				got[0] = "mutant"
				if mustStrSlice(t, in)[0] != "a" {
					t.Fatal("CloneValue shares []string")
				}
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.check(t, tc.in, CloneValue(tc.in))
		})
	}
}
