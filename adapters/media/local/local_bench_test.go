package local

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/zenta-dev/zever/core/media"
)

// benchMedia opens a local backend rooted at a benchmark temp dir.
func benchMedia(b *testing.B) media.Media {
	b.Helper()

	m, err := New(media.Options{Root: b.TempDir(), DerivedTTL: media.DefaultDerivedTTL})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = m.Close() })

	return m
}

// benchPNG renders a small deterministic PNG payload.
func benchPNG(b *testing.B, w, h int) []byte {
	b.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		for y := range h {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xff})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		b.Fatalf("png.Encode: %v", err)
	}

	return buf.Bytes()
}

// BenchmarkUpload measures id generation, extension resolution, and write.
func BenchmarkUpload(b *testing.B) {
	m := benchMedia(b)
	ctx := b.Context()
	data := benchPNG(b, 32, 32)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := m.Upload(ctx, "a.png", data, media.UploadOptions{}); err != nil {
			b.Fatalf("Upload: %v", err)
		}
	}
}

// BenchmarkDownload measures id validation, glob lookup, and read.
func BenchmarkDownload(b *testing.B) {
	m := benchMedia(b)
	ctx := b.Context()
	data := benchPNG(b, 32, 32)

	asset, err := m.Upload(ctx, "a.png", data, media.UploadOptions{})
	if err != nil {
		b.Fatalf("Upload: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := m.Download(ctx, asset.ID); err != nil {
			b.Fatalf("Download: %v", err)
		}
	}
}

// BenchmarkStat measures id validation, glob lookup, and stat.
func BenchmarkStat(b *testing.B) {
	m := benchMedia(b)
	ctx := b.Context()
	data := benchPNG(b, 32, 32)

	asset, err := m.Upload(ctx, "a.png", data, media.UploadOptions{})
	if err != nil {
		b.Fatalf("Upload: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := m.Stat(ctx, asset.ID); err != nil {
			b.Fatalf("Stat: %v", err)
		}
	}
}
