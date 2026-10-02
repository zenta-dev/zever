// Package flagtest provides the conformance kit third-party flag adapters run to prove backend parity.
package flagtest

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/flag"
)

// Conformance verifies factory-built flags implement the flag.Flag
// contract: open/register round-trip, missing-key fallback with nil
// error for Bool/String/Int/JSON, invalid-key sentinels, and Close.
// Each subtest takes a fresh instance from factory so cases stay
// isolated. Tests never call time.Sleep and never touch the network.
//
// Seeded-value coverage (present keys return stored values,
// type-mismatch errors) lives in adapter tests: the kit factory
// supplies an unseeded client (e.g. static with an empty path), so
// the kit proves the fallback half every adapter must honor.
// Documented stub exemption: none; even an empty static set must
// return fallbacks with nil errors.
func Conformance(t *testing.T, factory func(t *testing.T) flag.Flag) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("Fallback", func(t *testing.T) { conformanceFallback(t, factory) })
	t.Run("InvalidKey", func(t *testing.T) { conformanceInvalidKey(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := flag.Open(flag.Adapter("conformance-missing-adapter"), flag.Options{}); !errors.Is(err, flag.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := flag.Adapter("conformance-probe-flag")

	if err := flag.Register(probe, nil); !errors.Is(err, flag.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(flag.Options) (flag.Flag, error) {
		return nil, errors.New("flagtest: probe factory must not run")
	}

	_ = flag.Register(probe, stub)

	if err := flag.Register(probe, stub); !errors.Is(err, flag.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceFallback(t *testing.T, factory func(t *testing.T) flag.Flag) {
	t.Helper()

	ctx := t.Context()
	f := factory(t)

	const missing = "conformance-definitely-missing-01"

	if got, err := f.Bool(ctx, missing, true); err != nil || !got {
		t.Errorf("Bool(missing) = %v,%v want true,nil", got, err)
	}

	if got, err := f.String(ctx, missing, "fallback"); err != nil || got != "fallback" {
		t.Errorf("String(missing) = %q,%v want fallback,nil", got, err)
	}

	if got, err := f.Int(ctx, missing, 42); err != nil || got != 42 {
		t.Errorf("Int(missing) = %v,%v want 42,nil", got, err)
	}

	var out struct {
		Enabled bool `json:"enabled"`
	}

	if err := f.JSON(ctx, missing, &out, map[string]any{"enabled": true}); err != nil {
		t.Errorf("JSON(missing) error = %v, want nil", err)
	}
}

func conformanceInvalidKey(t *testing.T, factory func(t *testing.T) flag.Flag) {
	t.Helper()

	ctx := t.Context()
	f := factory(t)

	if _, err := f.Bool(ctx, "", false); !errors.Is(err, flag.ErrInvalidKey) {
		t.Errorf("Bool(empty) err = %v, want ErrInvalidKey", err)
	}

	if _, err := f.String(ctx, "", ""); !errors.Is(err, flag.ErrInvalidKey) {
		t.Errorf("String(empty) err = %v, want ErrInvalidKey", err)
	}

	if _, err := f.Int(ctx, "", 0); !errors.Is(err, flag.ErrInvalidKey) {
		t.Errorf("Int(empty) err = %v, want ErrInvalidKey", err)
	}

	var out any

	if err := f.JSON(ctx, "", &out, nil); !errors.Is(err, flag.ErrInvalidKey) {
		t.Errorf("JSON(empty) err = %v, want ErrInvalidKey", err)
	}

	long := strings.Repeat("k", 257)
	if _, err := f.Bool(ctx, long, false); !errors.Is(err, flag.ErrInvalidKey) {
		t.Errorf("Bool(long) err = %v, want ErrInvalidKey", err)
	}

	if err := flag.ValidateKey(""); !errors.Is(err, flag.ErrInvalidKey) {
		t.Errorf("ValidateKey(empty) err = %v, want ErrInvalidKey", err)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) flag.Flag) {
	t.Helper()

	f := factory(t)

	if err := f.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := f.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
