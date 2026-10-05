package local

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/zenta-dev/zever/core/media"
)

func benchAdapter(b *testing.B) *adapter {
	b.Helper()
	m, err := New(media.Options{Root: b.TempDir()})
	if err != nil {
		b.Fatalf("New err = %v", err)
	}
	b.Cleanup(func() { _ = m.Close() })
	a, ok := m.(*adapter)
	if !ok {
		b.Fatal("New did not return *adapter")
	}
	return a
}

func benchPNG(b *testing.B) []byte {
	b.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 64))); err != nil {
		b.Fatalf("png.Encode err = %v", err)
	}
	return buf.Bytes()
}

func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m, err := New(media.Options{Root: b.TempDir()})
		if err != nil {
			b.Fatalf("New err = %v", err)
		}
		if err := m.Close(); err != nil {
			b.Fatalf("Close err = %v", err)
		}
	}
}

func BenchmarkUpload(b *testing.B) {
	a := benchAdapter(b)
	data := []byte("bench payload")
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := a.Upload(ctx, "f.txt", data, media.UploadOptions{ContentType: "text/plain"}); err != nil {
			b.Fatalf("Upload err = %v", err)
		}
	}
}

func BenchmarkDownload(b *testing.B) {
	a := benchAdapter(b)
	ctx := b.Context()
	asset, err := a.Upload(ctx, "f.txt", []byte("bench payload"), media.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		b.Fatalf("Upload err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := a.Download(ctx, asset.ID); err != nil {
			b.Fatalf("Download err = %v", err)
		}
	}
}

func BenchmarkUploadDelete(b *testing.B) {
	a := benchAdapter(b)
	data := []byte("bench payload")
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		asset, err := a.Upload(ctx, "f.txt", data, media.UploadOptions{ContentType: "text/plain"})
		if err != nil {
			b.Fatalf("Upload err = %v", err)
		}
		if err := a.Delete(ctx, asset.ID); err != nil {
			b.Fatalf("Delete err = %v", err)
		}
	}
}

func BenchmarkStat(b *testing.B) {
	a := benchAdapter(b)
	ctx := b.Context()
	asset, err := a.Upload(ctx, "f.txt", []byte("bench payload"), media.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		b.Fatalf("Upload err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := a.Stat(ctx, asset.ID); err != nil {
			b.Fatalf("Stat err = %v", err)
		}
	}
}

func BenchmarkProbeImage(b *testing.B) {
	a := benchAdapter(b)
	ctx := b.Context()
	asset, err := a.Upload(ctx, "img.png", benchPNG(b), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		b.Fatalf("Upload err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := a.Probe(ctx, asset.ID); err != nil {
			b.Fatalf("Probe err = %v", err)
		}
	}
}

func BenchmarkTransformImage(b *testing.B) {
	a := benchAdapter(b)
	ctx := b.Context()
	asset, err := a.Upload(ctx, "img.png", benchPNG(b), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		b.Fatalf("Upload err = %v", err)
	}
	ops := media.TransformOps{Width: "32", Height: "32"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := a.Transform(ctx, asset.ID, ops); err != nil {
			b.Fatalf("Transform err = %v", err)
		}
	}
}
