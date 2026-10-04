package s3

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// BenchmarkResizeNN measures nearest-neighbor scaling.
func BenchmarkResizeNN(b *testing.B) {
	src := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for x := range 64 {
		for y := range 64 {
			src.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x40, A: 0xff})
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = resizeNN(src, 32, 32)
	}
}

// BenchmarkTargetSize measures dimension resolution and proportional scaling.
func BenchmarkTargetSize(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, _, err := targetSize(1920, 1080, "640", ""); err != nil {
			b.Fatalf("targetSize: %v", err)
		}
	}
}

// BenchmarkReadCapped measures capped body draining.
func BenchmarkReadCapped(b *testing.B) {
	data := []byte(strings.Repeat("x", 1024))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := readCapped(bytes.NewReader(data), int64(len(data))); err != nil {
			b.Fatalf("readCapped: %v", err)
		}
	}
}

// BenchmarkProbeImage measures image header decode and pixel-count gating.
func BenchmarkProbeImage(b *testing.B) {
	d := &driver{maxPixels: 1 << 20}

	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		b.Fatalf("png.Encode: %v", err)
	}

	data := buf.Bytes()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := d.probeImage(data, ".png"); err != nil {
			b.Fatalf("probeImage: %v", err)
		}
	}
}
