package embed

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/zenta-dev/zever/core/i18n"
)

// TestEdgeEmptyCatalog checks behavior when no locale catalogs are present.
func TestEdgeEmptyCatalog(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{"readme.txt": {Data: []byte("not a catalog")}}

	be, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fsys, Dir: "."}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = be.Close() })

	locales, err := be.Locales(t.Context())
	if err != nil {
		t.Fatalf("Locales() error = %v", err)
	}

	if len(locales) != 0 {
		t.Fatalf("Locales() = %v, want empty", locales)
	}

	if _, err := be.Translate(t.Context(), "en", "hello", nil); !errors.Is(err, i18n.ErrKeyNotFound) {
		t.Fatalf("Translate(empty catalog) = %v, want ErrKeyNotFound", err)
	}
}

// TestEdgeTranslatePlainTemplateNoArgs checks a template with no actions
// renders with nil args.
func TestEdgeTranslatePlainTemplateNoArgs(t *testing.T) {
	t.Parallel()

	be := mustNew(t, testFS(), "en")

	got, err := be.Translate(t.Context(), "en", "bye", nil)
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}

	if got != "Goodbye" {
		t.Fatalf("Translate() = %q, want %q", got, "Goodbye")
	}
}

// TestEdgeTranslateWhitespaceLocale checks that a whitespace-padded locale is
// trimmed before lookup.
func TestEdgeTranslateWhitespaceLocale(t *testing.T) {
	t.Parallel()

	be := mustNew(t, testFS(), "en")

	got, err := be.Translate(t.Context(), "  en  ", "bye", nil)
	if err != nil {
		t.Fatalf("Translate() error = %v", err)
	}

	if got != "Goodbye" {
		t.Fatalf("Translate() = %q, want %q", got, "Goodbye")
	}
}

// TestEdgeTranslateUnknownKeyFields checks the typed not-found error fields.
func TestEdgeTranslateUnknownKeyFields(t *testing.T) {
	t.Parallel()

	be := mustNew(t, testFS(), "en")

	_, err := be.Translate(t.Context(), "en", "missing", nil)

	var knf i18n.KeyNotFoundError
	if !errors.As(err, &knf) {
		t.Fatalf("Translate() error = %T, want *KeyNotFoundError", err)
	}

	if knf.Locale != "en" || knf.Key != "missing" {
		t.Fatalf("KeyNotFoundError = %+v, want Locale=en Key=missing", knf)
	}
}

// TestEdgeLocalesAfterClose checks that listing locales after Close reports
// ErrClosed.
func TestEdgeLocalesAfterClose(t *testing.T) {
	t.Parallel()

	be := mustNew(t, testFS(), "en")

	if err := be.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if _, err := be.Locales(t.Context()); !errors.Is(err, i18n.ErrClosed) {
		t.Fatalf("Locales() after Close = %v, want ErrClosed", err)
	}
}
