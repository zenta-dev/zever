package embed

import (
	"testing"

	"github.com/zenta-dev/zever/core/i18n"
)

// benchEmbed builds an embedded-catalog backend over the test catalogs.
func benchEmbed(b *testing.B) i18n.I18n {
	b.Helper()

	be, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: testFS(), Dir: ".", Fallback: "en"}})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	b.Cleanup(func() { _ = be.Close() })

	return be
}

// BenchmarkNew measures loading and parsing the catalogs.
func BenchmarkNew(b *testing.B) {
	opts := i18n.Options{Embed: i18n.EmbedOptions{FS: testFS(), Dir: ".", Fallback: "en"}}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		be, err := New(opts)
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		_ = be.Close()
	}
}

// BenchmarkTranslate measures an exact-locale template execution.
func BenchmarkTranslate(b *testing.B) {
	be := benchEmbed(b)
	ctx := b.Context()
	args := map[string]string{"name": "Ada"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := be.Translate(ctx, "en", "hello", args); err != nil {
			b.Fatalf("Translate() error = %v", err)
		}
	}
}

// BenchmarkTranslateFallback measures a per-key fallback to the base locale.
func BenchmarkTranslateFallback(b *testing.B) {
	be := benchEmbed(b)
	ctx := b.Context()
	args := map[string]string{"name": "Ada"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := be.Translate(ctx, "fr", "bye", args); err != nil {
			b.Fatalf("Translate() error = %v", err)
		}
	}
}

// BenchmarkLocales measures listing and sorting the available locales.
func BenchmarkLocales(b *testing.B) {
	be := benchEmbed(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := be.Locales(ctx); err != nil {
			b.Fatalf("Locales() error = %v", err)
		}
	}
}

// BenchmarkValidLocaleName measures locale filename validation.
func BenchmarkValidLocaleName(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = validLocaleName("zh-Hant")
	}
}

// BenchmarkOpen measures a registry lookup plus construction via Open.
func BenchmarkOpen(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		be, openErr := New(i18n.Options{})
		if openErr != nil {
			b.Fatalf("Open() error = %v", openErr)
		}

		if closeErr := be.Close(); closeErr != nil {
			b.Fatalf("Close() error = %v", closeErr)
		}
	}
}
