package i18ntest_test

import (
	"testing"
	"testing/fstest"

	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/i18n/i18ntest"
)

// TestConformanceEmbed proves the kit passes against the embed adapter.
func TestConformanceEmbed(t *testing.T) {
	t.Parallel()

	i18ntest.Conformance(t, func(t *testing.T) i18n.I18n {
		t.Helper()

		b, err := i18nembed.New(i18n.Options{Embed: i18n.EmbedOptions{
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
