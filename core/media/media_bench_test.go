package media

import "testing"

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func BenchmarkRegister(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := Register(freshAdapter(), func(Options) (Media, error) { return &stubMedia{}, nil }); err != nil {
			b.Fatalf("Register err = %v", err)
		}
	}
}

func BenchmarkOpen(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Media, error) { return &stubMedia{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Open(a, Options{}); err != nil {
			b.Fatalf("Open err = %v", err)
		}
	}
}

func BenchmarkOpenParallel(b *testing.B) {
	a := freshAdapter()
	if err := Register(a, func(Options) (Media, error) { return &stubMedia{}, nil }); err != nil {
		b.Fatalf("Register err = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := Open(a, Options{}); err != nil {
				b.Fatalf("Open err = %v", err)
			}
		}
	})
}

func BenchmarkParseAdapter(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseAdapter("s3")
	}
}

func BenchmarkValidID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !ValidID("asset-123_abc") {
			b.Fatal("ValidID = false, want true")
		}
	}
}

func BenchmarkValidHexID(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !ValidHexID("0123456789abcdef0123456789abcdef") {
			b.Fatal("ValidHexID = false, want true")
		}
	}
}

func BenchmarkGenerateIDWithReader(b *testing.B) {
	r := zeroReader{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GenerateIDWithReader(r); err != nil {
			b.Fatalf("GenerateIDWithReader err = %v", err)
		}
	}
}

func BenchmarkExtForContentType(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ExtForContentType("image/png; charset=binary")
	}
}

func BenchmarkContentTypeForExt(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ContentTypeForExt(".png")
	}
}

func BenchmarkKindForExt(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = KindForExt("mp4")
	}
}
