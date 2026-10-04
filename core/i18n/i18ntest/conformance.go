// Package i18ntest provides the conformance kit third-party i18n adapters run to prove backend parity.
package i18ntest

import (
	"errors"
	"slices"
	"testing"

	"github.com/zenta-dev/zever/core/i18n"
)

// Conformance verifies factory-built backends implement the i18n.I18n
// contract: open/register round-trip, template translation, missing-
// key sentinel, locale listing, and Close. Each subtest takes a fresh
// instance from factory so cases stay isolated. Tests never call
// time.Sleep and never touch the network.
//
// Fixture contract: the factory must seed locale "en" with key
// "hello" rendering to "Hello, <name>!" for args {"name": <name>}
// (for example catalog entry "Hello, {{.name}}!"). Missing-key
// probes use key "missing-kit-key" in the seeded "en" locale so both
// embedded and remote backends report ErrKeyNotFound rather than a
// locale-level sentinel.
func Conformance(t *testing.T, factory func(t *testing.T) i18n.I18n) {
	t.Helper()

	t.Run("OpenRegister", func(t *testing.T) { conformanceOpenRegister(t) })
	t.Run("Translate", func(t *testing.T) { conformanceTranslate(t, factory) })
	t.Run("MissingKey", func(t *testing.T) { conformanceMissingKey(t, factory) })
	t.Run("Locales", func(t *testing.T) { conformanceLocales(t, factory) })
	t.Run("Close", func(t *testing.T) { conformanceClose(t, factory) })
}

func conformanceOpenRegister(t *testing.T) {
	t.Helper()

	if _, err := i18n.Open(i18n.Adapter("conformance-missing-adapter"), i18n.Options{}); !errors.Is(err, i18n.ErrUnknownAdapter) {
		t.Fatalf("Open(missing) err = %v, want ErrUnknownAdapter", err)
	}

	probe := i18n.Adapter("conformance-probe-i18n")

	if err := i18n.Register(probe, nil); !errors.Is(err, i18n.ErrNilFactory) {
		t.Fatalf("Register(nil) err = %v, want ErrNilFactory", err)
	}

	stub := func(i18n.Options) (i18n.I18n, error) {
		return nil, errors.New("i18ntest: probe factory must not run")
	}

	_ = i18n.Register(probe, stub)

	if err := i18n.Register(probe, stub); !errors.Is(err, i18n.ErrDuplicate) {
		t.Fatalf("Register(duplicate) err = %v, want ErrDuplicate", err)
	}
}

func conformanceTranslate(t *testing.T, factory func(t *testing.T) i18n.I18n) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	got, err := b.Translate(ctx, "en", "hello", map[string]string{"name": "Ada"})
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}

	if got != "Hello, Ada!" {
		t.Errorf("Translate() = %q, want %q", got, "Hello, Ada!")
	}
}

func conformanceMissingKey(t *testing.T, factory func(t *testing.T) i18n.I18n) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	if _, err := b.Translate(ctx, "en", "missing-kit-key", nil); !errors.Is(err, i18n.ErrKeyNotFound) {
		t.Errorf("Translate(missing key) err = %v, want ErrKeyNotFound", err)
	}

	var keyErr i18n.KeyNotFoundError
	if _, err := b.Translate(ctx, "en", "missing-kit-key", nil); !errors.As(err, &keyErr) {
		t.Errorf("errors.As(err, KeyNotFoundError) = false (err = %T %v)", err, err)
	}
}

func conformanceLocales(t *testing.T, factory func(t *testing.T) i18n.I18n) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	locales, err := b.Locales(ctx)
	if err != nil {
		t.Fatalf("Locales() error = %v", err)
	}

	if !slices.Contains(locales, "en") {
		t.Errorf("Locales() = %q, want to contain en", locales)
	}
}

func conformanceClose(t *testing.T, factory func(t *testing.T) i18n.I18n) {
	t.Helper()

	ctx := t.Context()
	b := factory(t)

	if err := b.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := b.Translate(ctx, "en", "hello", nil); !errors.Is(err, i18n.ErrClosed) {
		t.Errorf("Translate(after Close) err = %v, want ErrClosed", err)
	}

	if err := b.Close(); err != nil {
		t.Errorf("Close() second error = %v, want nil", err)
	}
}
