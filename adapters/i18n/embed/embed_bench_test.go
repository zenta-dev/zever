package embed

import (
	"testing"
	"testing/fstest"

	"github.com/zenta-dev/zever/core/i18n"
)

// benchI18n builds a catalog-backed driver with an en fallback.
func benchI18n(b *testing.B) i18n.I18n {
	b.Helper()

	fsys := fstest.MapFS{
		"en.json": {Data: []byte(`{"hello":"Hello, {{.name}}!","bye":"Goodbye"}`)},
		"fr.json": {Data: []byte(`{"hello":"Bonjour, {{.name}}!"}`)},
	}

	ii, err := New(i18n.Options{Embed: i18n.EmbedOptions{FS: fsys, Dir: ".", Fallback: "en"}})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = ii.Close() })

	return ii
}

// BenchmarkTranslate measures template lookup and execution.
func BenchmarkTranslate(b *testing.B) {
	ii := benchI18n(b)
	ctx := b.Context()
	args := map[string]string{"name": "Ada"}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := ii.Translate(ctx, "en", "hello", args); err != nil {
			b.Fatalf("Translate: %v", err)
		}
	}
}

// BenchmarkTranslateParallel measures concurrent lookups (read-locked).
func BenchmarkTranslateParallel(b *testing.B) {
	ii := benchI18n(b)
	ctx := b.Context()
	args := map[string]string{"name": "Ada"}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := ii.Translate(ctx, "en", "hello", args); err != nil {
				b.Fatalf("Translate: %v", err)
			}
		}
	})
}

// BenchmarkLocales measures catalog enumeration and sorting.
func BenchmarkLocales(b *testing.B) {
	ii := benchI18n(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := ii.Locales(ctx); err != nil {
			b.Fatalf("Locales: %v", err)
		}
	}
}
