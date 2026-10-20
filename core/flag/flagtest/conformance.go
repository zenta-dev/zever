// Package flagtest provides the conformance kit third-party flag adapters run to prove backend parity.
package flagtest

import (
	"context"
	"errors"
	"fmt"
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
// errf builds a non-wrapping descriptive error with Sprintf semantics.
// Check helpers use it (instead of fmt.Errorf with %w) for diagnostics
// where the formatted error may be nil: %w of a nil error prints
// "%!w(<nil>)", diverging from the historical Fatalf text, and errorlint
// forbids %v of an error in Errorf. Failure text stays byte-identical.
// It deliberately avoids fmt.Errorf so only real failures wrap.
func errf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)

	return errors.New(msg)
}

func Conformance(t *testing.T, factory func(t *testing.T) flag.Flag) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("Fallback", func(t *testing.T) { conformanceFallback(t, factory) })
	t.Run("InvalidKey", func(t *testing.T) { conformanceInvalidKey(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if err := checkOpenRegister(); err != nil {
		t.Fatal(err)
	}
}

// checkOpenRegister proves the open/register round-trip against the
// shared registry. It returns a descriptive error on the first
// contract violation so unit tests can drive every branch.
func checkOpenRegister() error {
	if _, err := flag.Open(flag.Adapter("conformance-missing-adapter"), flag.Options{}); !errors.Is(err, flag.ErrUnknownAdapter) {
		return fmt.Errorf("flagtest: Open(missing) err = %w, want ErrUnknownAdapter", err)
	}

	probe := flag.Adapter("conformance-probe-flag")

	if err := flag.Register(probe, nil); !errors.Is(err, flag.ErrNilFactory) {
		return fmt.Errorf("flagtest: Register(nil) err = %w, want ErrNilFactory", err)
	}

	stub := func(flag.Options) (flag.Flag, error) {
		return nil, errors.New("flagtest: probe factory must not run")
	}

	_ = flag.Register(probe, stub)

	if err := flag.Register(probe, stub); !errors.Is(err, flag.ErrDuplicate) {
		return fmt.Errorf("flagtest: Register(duplicate) err = %w, want ErrDuplicate", err)
	}

	return nil
}

func conformanceFallback(t *testing.T, factory func(t *testing.T) flag.Flag) {
	t.Helper()

	if err := checkFallback(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkFallback proves missing keys return fallbacks with nil errors.
// Soft mismatches join into one error so every violation is reported.
func checkFallback(ctx context.Context, f flag.Flag) error {
	const missing = "conformance-definitely-missing-01"

	var errs []error

	if got, err := f.Bool(ctx, missing, true); err != nil || !got {
		errs = append(errs, errf("Bool(missing) = %v,%v want true,nil", got, err))
	}

	if got, err := f.String(ctx, missing, "fallback"); err != nil || got != "fallback" {
		errs = append(errs, errf("String(missing) = %q,%v want fallback,nil", got, err))
	}

	if got, err := f.Int(ctx, missing, 42); err != nil || got != 42 {
		errs = append(errs, errf("Int(missing) = %v,%v want 42,nil", got, err))
	}

	var out struct {
		Enabled bool `json:"enabled"`
	}

	if err := f.JSON(ctx, missing, &out, map[string]any{"enabled": true}); err != nil {
		errs = append(errs, errf("JSON(missing) error = %v, want nil", err))
	}

	return errors.Join(errs...)
}

func conformanceInvalidKey(t *testing.T, factory func(t *testing.T) flag.Flag) {
	t.Helper()

	if err := checkInvalidKey(t.Context(), factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkInvalidKey proves empty and overlong keys fail closed with
// ErrInvalidKey. Soft mismatches join so every method is reported.
func checkInvalidKey(ctx context.Context, f flag.Flag) error {
	var errs []error

	if _, err := f.Bool(ctx, "", false); !errors.Is(err, flag.ErrInvalidKey) {
		errs = append(errs, errf("Bool(empty) err = %v, want ErrInvalidKey", err))
	}

	if _, err := f.String(ctx, "", ""); !errors.Is(err, flag.ErrInvalidKey) {
		errs = append(errs, errf("String(empty) err = %v, want ErrInvalidKey", err))
	}

	if _, err := f.Int(ctx, "", 0); !errors.Is(err, flag.ErrInvalidKey) {
		errs = append(errs, errf("Int(empty) err = %v, want ErrInvalidKey", err))
	}

	var out any

	if err := f.JSON(ctx, "", &out, nil); !errors.Is(err, flag.ErrInvalidKey) {
		errs = append(errs, errf("JSON(empty) err = %v, want ErrInvalidKey", err))
	}

	long := strings.Repeat("k", 257)
	if _, err := f.Bool(ctx, long, false); !errors.Is(err, flag.ErrInvalidKey) {
		errs = append(errs, errf("Bool(long) err = %v, want ErrInvalidKey", err))
	}

	if err := flag.ValidateKey(""); !errors.Is(err, flag.ErrInvalidKey) {
		errs = append(errs, errf("ValidateKey(empty) err = %v, want ErrInvalidKey", err))
	}

	return errors.Join(errs...)
}

func conformanceClose(t *testing.T, factory func(t *testing.T) flag.Flag) {
	t.Helper()

	if err := checkClose(factory(t)); err != nil {
		t.Fatal(err)
	}
}

// checkClose proves Close is idempotent: first and second calls return nil.
func checkClose(f flag.Flag) error {
	if err := f.Close(); err != nil {
		return fmt.Errorf("flagtest: Close() error = %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("flagtest: Close() second error = %w, want nil", err)
	}

	return nil
}
