package media

import "testing"

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func BenchmarkValidID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if !ValidID("asset-123_abc") {
			b.Fatal("ValidID = false, want true")
		}
	}
}

func BenchmarkValidHexID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if !ValidHexID("0123456789abcdef0123456789abcdef") {
			b.Fatal("ValidHexID = false, want true")
		}
	}
}

func BenchmarkGenerateIDWithReader(b *testing.B) {
	r := zeroReader{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := GenerateIDWithReader(r); err != nil {
			b.Fatalf("GenerateIDWithReader err = %v", err)
		}
	}
}

func BenchmarkExtForContentType(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ExtForContentType("image/png; charset=binary")
	}
}

func BenchmarkContentTypeForExt(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ContentTypeForExt(".png")
	}
}

func BenchmarkKindForExt(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = KindForExt("mp4")
	}
}
