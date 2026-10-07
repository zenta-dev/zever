package s3opts

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/core/storage"
)

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

// newBenchCore builds a Core with a stub transport and the given default
// policy for benchmarking.
func newBenchCore(b *testing.B, def *storage.Policy) *Core {
	b.Helper()

	var store storage.PolicyStore
	if err := store.ResolveFromConfig(&storage.PolicyConfig{Default: def}); err != nil {
		b.Fatalf("ResolveFromConfig() error = %v", err)
	}

	cfg, err := config.LoadDefaultConfig(b.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("ak", "sk", "")),
	)
	if err != nil {
		b.Fatalf("LoadDefaultConfig() error = %v", err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
		o.HTTPClient = &stubTransport{fn: boomStub}
		o.Retryer = aws.NopRetryer{}
	})

	return New("test", "us-east-1", "", client, s3.NewPresignClient(client), store, nil)
}

// BenchmarkPresignUploadPublic measures the public-policy upload path, which
// resolves a static URL without any S3 call.
func BenchmarkPresignUploadPublic(b *testing.B) {
	ctx := b.Context()
	// Public upload requires both Write and Update to be public.
	c := newBenchCore(b, publicUploadPolicy())

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := c.PresignUpload(ctx, "bkt", "media/photo.jpg", "image/jpeg", time.Minute); err != nil {
			b.Fatalf("PresignUpload() error = %v", err)
		}
	}
}

// BenchmarkPresignDownloadPublic measures the public-policy download path,
// which resolves a static URL without any S3 call.
func BenchmarkPresignDownloadPublic(b *testing.B) {
	ctx := b.Context()
	pol := &storage.Policy{Read: storage.Rule{Public: true}}
	c := newBenchCore(b, pol)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := c.PresignDownload(ctx, "bkt", "media/photo.jpg", time.Minute); err != nil {
			b.Fatalf("PresignDownload() error = %v", err)
		}
	}
}
