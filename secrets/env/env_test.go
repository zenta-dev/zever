package env

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/secrets"
)

func TestNew_requiresPrefix(t *testing.T) {
	t.Parallel()

	if _, err := New(Options{}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNew_valid(t *testing.T) {
	t.Parallel()

	s, err := New(Options{Prefix: "TNEW_"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if s == nil {
		t.Fatal("New returned nil")
	}
}

func TestGet_existing(t *testing.T) {
	t.Setenv("TENV_GREETING", "hello")

	a, err := New(Options{Prefix: "TENV_"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	val, err := a.Get(context.Background(), "GREETING")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if string(val) != "hello" {
		t.Fatalf("got %q, want %q", val, "hello")
	}
}

func TestGet_missing_errNotFound(t *testing.T) {
	a, err := New(Options{Prefix: "TENV_MISSING_"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = a.Get(context.Background(), "NOPE_DEF_MISSING")
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGet_invalidName_errInvalidKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"slash", "a/b"},
		{"dotdot", "a..b"},
		{"space", "a b"},
		{"exclaim", "a!b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a, err := New(Options{Prefix: "TENV_"})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			if _, err := a.Get(context.Background(), tt.input); !errors.Is(err, secrets.ErrInvalidKey) {
				t.Fatalf("err = %v, want ErrInvalidKey", err)
			}
		})
	}
}

func TestGet_enforcesPrefixBoundary(t *testing.T) {
	t.Setenv("TBOUND_K", "v")

	a, err := New(Options{Prefix: "TBOUND"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	val, err := a.Get(context.Background(), "K")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if string(val) != "v" {
		t.Fatalf("got %q, want %q", val, "v")
	}

	if _, err := a.Get(context.Background(), "K2"); err == nil {
		t.Fatal("expected boundary error, got nil")
	}
}

func TestSet_notSupported(t *testing.T) {
	t.Parallel()

	a, err := New(Options{Prefix: "TENV_"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := a.Set(context.Background(), "k", []byte("v")); !errors.Is(err, secrets.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}
}

func TestDelete_notSupported(t *testing.T) {
	t.Parallel()

	a, err := New(Options{Prefix: "TENV_"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := a.Delete(context.Background(), "k"); !errors.Is(err, secrets.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}
}

func TestList_filtersPrefix(t *testing.T) {
	t.Setenv("TLIST_A", "1")
	t.Setenv("TLIST_B", "2")

	a, err := New(Options{Prefix: "TLIST_"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	keys, err := a.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := map[string]bool{}
	for _, k := range keys {
		found[k] = true
	}

	if !found["A"] || !found["B"] {
		t.Fatalf("want A and B, got %v", keys)
	}
}

func TestList_enforcesBoundary(t *testing.T) {
	t.Setenv("TBND_A", "1")
	t.Setenv("TBND_B", "2")
	t.Setenv("TBNDAB_C", "3")
	t.Setenv("TBND_", "4")
	t.Setenv("TBND", "5")

	a, err := New(Options{Prefix: "TBND"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	keys, err := a.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	found := map[string]bool{}
	for _, k := range keys {
		found[k] = true
	}

	if !found["A"] || !found["B"] {
		t.Fatalf("want A and B, got %v", keys)
	}

	if found["AB_C"] || found[""] {
		t.Fatalf("boundary-less keys leaked: %v", keys)
	}
}

func TestClose_nilError(t *testing.T) {
	t.Parallel()

	a, err := New(Options{Prefix: "TENV_"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := a.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestBoundary_suffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		prefix string
		want   string
	}{
		{"empty", "", ""},
		{"with underscore", "APP_", "APP_"},
		{"without underscore", "APP", "APP_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := &adapter{prefix: tt.prefix}
			if got := a.boundary(); got != tt.want {
				t.Fatalf("boundary = %q, want %q", got, tt.want)
			}
		})
	}
}
