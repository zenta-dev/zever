package document

import (
	"testing"
	"time"
)

// BenchmarkRegister measures registry insertion for a fresh adapter key.
func BenchmarkOptionsValidate(b *testing.B) {
	opts := Options{
		Timeout:        time.Second,
		Quality:        80,
		DPI:            150,
		MaxOutputBytes: 1 << 20,
		Endpoint:       "https://render.example.com",
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := opts.Validate(); err != nil {
			b.Fatalf("Validate err = %v", err)
		}
	}
}

// BenchmarkClampQuality measures quality clamping.
func BenchmarkClampQuality(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = ClampQuality(150)
	}
}

// BenchmarkAdapterString measures canonical adapter naming.
func BenchmarkRender(b *testing.B) {
	doc := &stubDocument{out: []byte("%PDF-1.4")}
	source := []byte("\\documentclass{article}")
	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if _, err := doc.Render(ctx, source, FormatPDF); err != nil {
			b.Fatalf("Render err = %v", err)
		}
	}
}
