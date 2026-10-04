package embed

import (
	"errors"
	"testing"
	"testing/fstest"

	"github.com/zenta-dev/zever/core/i18n"
)

// TestEdgeTranslate_nilArgs proves a message with no placeholders renders
// with a nil args map.
func TestEdgeTranslate_nilArgs(t *testing.T) {
	t.Parallel()

	be := mustNew(t, testFS(), "en")

	got, err := be.Translate(t.Context(), "en", "bye", nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}

	if got != "Goodbye" {
		t.Fatalf("Translate = %q, want Goodbye", got)
	}
}

// TestEdgeTranslate_missingKey proves an unknown key yields
// KeyNotFoundError rather than a fallback locale's value.
func TestEdgeTranslate_missingKey(t *testing.T) {
	t.Parallel()

	be := mustNew(t, testFS(), "en")

	_, err := be.Translate(t.Context(), "fr", "does-not-exist", nil)

	var knf *i18n.KeyNotFoundError
	if !errors.As(err, &knf) {
		t.Fatalf("Translate err = %v, want *KeyNotFoundError", err)
	}
}

// TestEdgeNew_emptyCatalog proves a catalog directory with no JSON files
// yields an adapter with zero locales.
func TestEdgeNew_emptyCatalog(t *testing.T) {
	t.Parallel()

	be, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fstest.MapFS{}, Dir: ".", Fallback: "en"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = be.Close() })

	locales, err := be.Locales(t.Context())
	if err != nil {
		t.Fatalf("Locales: %v", err)
	}

	if len(locales) != 0 {
		t.Fatalf("Locales = %v, want empty", locales)
	}
}
