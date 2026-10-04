package s3opts

import "testing"

// BenchmarkEscapeKey measures segment-wise key escaping.
func BenchmarkEscapeKey(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = EscapeKey("media/2026/photo one.jpg")
	}
}

// BenchmarkStaticURL measures building the public URL from a Core.
func BenchmarkStaticURL(b *testing.B) {
	c := &Core{Region: DefaultRegion}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := c.StaticURL("bucket", "media/photo.jpg"); err != nil {
			b.Fatalf("StaticURL() error = %v", err)
		}
	}
}

// BenchmarkWithDefaults measures trimming and region defaulting.
func BenchmarkWithDefaults(b *testing.B) {
	cfg := Options{Endpoint: "  https://s3.example.com  ", Region: "  ", Bucket: "  b  ", AccessKeyID: "  ak  ", SecretAccessKey: "  sk  "}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = cfg.WithDefaults("")
	}
}

// BenchmarkValidate measures the joined validation pass.
func BenchmarkValidate(b *testing.B) {
	cfg := Options{Endpoint: "https://s3.example.com", Region: "us-east-1", Bucket: "b", AccessKeyID: "ak", SecretAccessKey: "sk"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := cfg.Validate(true, true); err != nil {
			b.Fatalf("Validate() error = %v", err)
		}
	}
}
