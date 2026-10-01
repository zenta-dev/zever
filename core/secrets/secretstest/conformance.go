// Package secretstest provides the conformance kit third-party secrets adapters run to prove backend parity.
package secretstest

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
)

// Conformance verifies factory-built secrets implement the secrets.Secrets
// contract: Get missing returns ErrNotFound, Set/Get round-trips values,
// List contains written keys, Delete of a missing key returns ErrNotFound,
// and ValidateName rejects traversal. Each subtest takes a fresh instance
// from factory so cases stay isolated. Tests are deterministic, use
// t.Context, never call time.Sleep, and never touch the network (vault
// fakes use httptest loopback).
//
// Capability split: read-only adapters (for example env) report
// ErrNotSupported from Set and Delete instead of storing. The Set/Get and
// List subtests accept ErrNotSupported from Set as a read-only signal and
// then assert the read-only contract (List succeeds without error) instead
// of the round-trip. The Delete subtest accepts either ErrNotFound
// (writable adapters) or ErrNotSupported (read-only adapters) for a
// missing key. Invalid-name checks against Set/Delete likewise accept
// either ErrInvalidKey or ErrNotSupported, while Get with an invalid name
// must always report ErrInvalidKey and secrets.ValidateName itself must
// report ErrInvalidKey.
func Conformance(t *testing.T, factory func(t *testing.T) secrets.Secrets) {
	t.Helper()

	t.Run("GetMissing", func(t *testing.T) { conformanceGetMissing(t, factory) })
	t.Run("SetGetRoundTrip", func(t *testing.T) { conformanceSetGet(t, factory) })
	t.Run("ListContains", func(t *testing.T) { conformanceList(t, factory) })
	t.Run("DeleteMissing", func(t *testing.T) { conformanceDelete(t, factory) })
	t.Run("ValidateName", func(t *testing.T) { conformanceValidateName(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceGetMissing(t *testing.T, factory func(t *testing.T) secrets.Secrets) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if _, err := s.Get(ctx, "zever-conformance-definitely-missing-01"); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get(missing) err = %v, want ErrNotFound", err)
	}
}

func conformanceSetGet(t *testing.T, factory func(t *testing.T) secrets.Secrets) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	const name = "conformance-roundtrip"

	val := []byte("conformance-value-01")
	if err := s.Set(ctx, name, val); err != nil {
		if errors.Is(err, secrets.ErrNotSupported) {
			return
		}
		t.Fatalf("Set() error = %v", err)
	}

	val[0] = 'X'

	got, err := s.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "conformance-value-01" {
		t.Fatalf("Get() = %q, want conformance-value-01 (stored copy)", got)
	}

	got[0] = 'Y'

	again, err := s.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(again) != "conformance-value-01" {
		t.Fatalf("Get() = %q, want conformance-value-01 (returned copy)", again)
	}

	if err := s.Set(ctx, name, []byte("conformance-value-02")); err != nil {
		t.Fatalf("Set() overwrite error = %v", err)
	}

	got, err = s.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != "conformance-value-02" {
		t.Fatalf("Get() = %q, want conformance-value-02", got)
	}
}

func conformanceList(t *testing.T, factory func(t *testing.T) secrets.Secrets) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	const name = "conformance-list-key"

	if err := s.Set(ctx, name, []byte("v")); err != nil {
		if errors.Is(err, secrets.ErrNotSupported) {
			keys, listErr := s.List(ctx)
			if listErr != nil {
				t.Fatalf("List() error = %v", listErr)
			}

			for _, k := range keys {
				if k == "" {
					t.Errorf("List() returned empty name in %q", keys)
				}
			}

			return
		}

		t.Fatalf("Set() error = %v", err)
	}

	keys, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	for _, k := range keys {
		if k == name {
			return
		}
	}

	t.Fatalf("List() = %q, want it to contain %q", keys, name)
}

func conformanceDelete(t *testing.T, factory func(t *testing.T) secrets.Secrets) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	err := s.Delete(ctx, "zever-conformance-definitely-missing-01")
	if err == nil {
		t.Fatalf("Delete(missing) = nil, want ErrNotFound or ErrNotSupported")
	}

	if !errors.Is(err, secrets.ErrNotFound) && !errors.Is(err, secrets.ErrNotSupported) {
		t.Fatalf("Delete(missing) err = %v, want ErrNotFound or ErrNotSupported", err)
	}

	const name = "conformance-delete-me"

	if err := s.Set(ctx, name, []byte("v")); err != nil {
		if errors.Is(err, secrets.ErrNotSupported) {
			return
		}
		t.Fatalf("Set() error = %v", err)
	}

	if err := s.Delete(ctx, name); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := s.Get(ctx, name); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Get(deleted) err = %v, want ErrNotFound", err)
	}

	if err := s.Delete(ctx, name); !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("Delete(again) err = %v, want ErrNotFound", err)
	}
}

func conformanceValidateName(t *testing.T, factory func(t *testing.T) secrets.Secrets) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	valid := []string{"my-secret", "db.password", "api_key_2024"}
	for _, name := range valid {
		if err := secrets.ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) error = %v, want nil", name, err)
		}
	}

	invalid := []string{"", "a/b", "../etc/passwd", "a..b", "a b", "a!b"}
	for _, name := range invalid {
		if err := secrets.ValidateName(name); !errors.Is(err, secrets.ErrInvalidKey) {
			t.Errorf("ValidateName(%q) err = %v, want ErrInvalidKey", name, err)
		}

		if _, err := s.Get(ctx, name); !errors.Is(err, secrets.ErrInvalidKey) {
			t.Errorf("Get(%q) err = %v, want ErrInvalidKey", name, err)
		}

		if err := s.Set(ctx, name, []byte("v")); !errors.Is(err, secrets.ErrInvalidKey) && !errors.Is(err, secrets.ErrNotSupported) {
			t.Errorf("Set(%q) err = %v, want ErrInvalidKey or ErrNotSupported", name, err)
		}

		if err := s.Delete(ctx, name); !errors.Is(err, secrets.ErrInvalidKey) && !errors.Is(err, secrets.ErrNotSupported) {
			t.Errorf("Delete(%q) err = %v, want ErrInvalidKey or ErrNotSupported", name, err)
		}
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) secrets.Secrets) {
	t.Helper()

	ctx := t.Context()
	s := factory(t)

	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := s.Close(ctx); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
