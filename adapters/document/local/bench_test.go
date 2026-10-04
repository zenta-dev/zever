package local

import (
	"testing"

	"github.com/zenta-dev/zever/core/document"
)

// BenchmarkRenderUnsupportedFormat measures the fast reject path that runs
// before any browser work.
func BenchmarkRenderUnsupportedFormat(b *testing.B) {
	d := &driver{tmpDir: "/tmp", timeout: document.DefaultTimeout}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := d.Render(ctx, []byte("<html></html>"), document.OutputFormat("bogus")); err == nil {
			b.Fatal("want error")
		}
	}
}

// BenchmarkNewClose measures allocator construction and teardown. It does not
// launch a browser: Render is the only path that does.
func BenchmarkNewClose(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		d, err := New(document.Options{})
		if err != nil {
			b.Fatal(err)
		}

		if err := d.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
