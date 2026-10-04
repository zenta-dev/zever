package latex

import (
	"testing"

	"github.com/zenta-dev/zever/core/document"
)

// benchSource is a minimal LaTeX document accepted by the stub compiler.
const benchSource = `\documentclass{article}\begin{document}hello\end{document}`

// BenchmarkRenderPDF measures the full compile path: temp dir, stub compiler
// invocation, PDF read, and output-size cap.
func BenchmarkRenderPDF(b *testing.B) {
	stubBin(b, document.DefaultLatexCommand, fakeTexOK)
	d := mustOpen(b, document.Options{})
	ctx := b.Context()
	src := []byte(benchSource)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := d.Render(ctx, src, document.FormatPDF); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderUnsupportedFormat measures the fast reject path for an
// unhandled output format.
func BenchmarkRenderUnsupportedFormat(b *testing.B) {
	d := &driver{}
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := d.Render(ctx, []byte("x"), document.OutputFormat("bogus")); err == nil {
			b.Fatal("want error")
		}
	}
}
