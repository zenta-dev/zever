package remote

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenta-dev/zever/core/document"
)

// benchPDF is a minimal PDF magic-prefixed body the server returns.
var benchPDF = []byte("%PDF-1.4 benchmark body")

// BenchmarkRender measures a full remote render round trip against an
// in-process HTTP render service.
func BenchmarkRender(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(benchPDF)
	}))
	b.Cleanup(srv.Close)

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		b.Fatal(err)
	}

	ctx := b.Context()
	src := []byte("<html></html>")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := doc.Render(ctx, src, document.FormatPDF); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRenderJSON measures the JSON-envelope response path.
func BenchmarkRenderJSON(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderResponse{Data: benchPDF})
	}))
	b.Cleanup(srv.Close)

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		b.Fatal(err)
	}

	ctx := b.Context()
	src := []byte("<html></html>")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := doc.Render(ctx, src, document.FormatPDF); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSniffBinaryMagic measures response-body magic detection.
func BenchmarkSniffBinaryMagic(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 64)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = sniffBinaryMagic(body)
	}
}
