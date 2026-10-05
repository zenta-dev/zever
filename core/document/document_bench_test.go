package document

import (
	"testing"
	"time"
)

// BenchmarkRegister measures registry insertion for a fresh adapter key.
func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if err := Register(freshAdapter(), func(Options) (Document, error) { return &stubDocument{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

// BenchmarkOpen measures validated registry lookup plus the factory call.
func BenchmarkOpen(b *testing.B) {
	adapter := freshAdapter()
	if err := Register(adapter, func(Options) (Document, error) { return &stubDocument{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := Open(adapter, Options{Timeout: time.Second}); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

// BenchmarkOptionsValidate measures the option range and endpoint checks.
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
func BenchmarkAdapterString(b *testing.B) {
	a := Latex

	b.ReportAllocs()

	for b.Loop() {
		_ = a.String()
	}
}

// BenchmarkParseAdapter measures adapter name parsing.
func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := ParseAdapter("latex"); err != nil {
			b.Fatalf("ParseAdapter err = %v", err)
		}
	}
}

// BenchmarkRender measures the Document contract dispatch through a stub.
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
