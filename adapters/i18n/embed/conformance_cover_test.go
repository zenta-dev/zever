package embed

import (
	"testing"
	"testing/fstest"

	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/i18n/i18ntest"
)

// TestEmbedConformance proves the embed adapter honors the i18n.I18n
// contract via the shared conformance kit. The catalog seeds the
// kit fixture locale (en/hello) from an in-memory FS (no network).
func TestEmbedConformance(t *testing.T) {
	t.Parallel()

	i18ntest.Conformance(t, func(t *testing.T) i18n.I18n {
		t.Helper()

		b, err := New(i18n.Options{Embed: i18n.EmbedOptions{
			FS: fstest.MapFS{
				"en.json": {Data: []byte(`{"hello":"Hello, {{.name}}!"}`)},
			},
			Dir: ".",
		}})
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		t.Cleanup(func() { _ = b.Close() })

		return b
	})
}
