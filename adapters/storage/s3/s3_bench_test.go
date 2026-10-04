package s3

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/storage"
)

// newBenchS3 opens an adapter with a custom endpoint and URL base so client
// construction stays offline (no network, no metadata lookups).
func newBenchS3(b *testing.B) *s3Adapter {
	b.Helper()

	s, err := New(storage.Options{
		URLBase: "https://cdn.example.com",
		S3Options: storage.S3Options{
			Endpoint:        "https://s3.example.com",
			Region:          "us-east-1",
			AccessKeyID:     "bench-key",
			SecretAccessKey: "bench-secret",
		},
	})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	a, ok := s.(*s3Adapter)
	if !ok {
		b.Fatalf("New() type = %T, want *s3Adapter", s)
	}

	return a
}

// BenchmarkPresignUpload measures local SigV4 signing of a PUT URL (no network
// round trip: the presigner computes the signature in-process).
func BenchmarkPresignUpload(b *testing.B) {
	a := newBenchS3(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := a.PresignUpload(ctx, "bucket", "dir/object.txt", "text/plain", time.Minute); err != nil {
			b.Fatalf("PresignUpload(): %v", err)
		}
	}
}

// BenchmarkPresignDownload measures local SigV4 signing of a GET URL.
func BenchmarkPresignDownload(b *testing.B) {
	a := newBenchS3(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := a.PresignDownload(ctx, "bucket", "dir/object.txt", time.Minute); err != nil {
			b.Fatalf("PresignDownload(): %v", err)
		}
	}
}

// BenchmarkBucketPolicyDoc measures rendering the managed bucket-policy
// document from a fully-public policy.
func BenchmarkBucketPolicyDoc(b *testing.B) {
	p := storage.Policy{
		Read:   storage.Rule{Public: true},
		Write:  storage.Rule{Public: true},
		Update: storage.Rule{Public: true},
		Delete: storage.Rule{Public: true},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if doc := bucketPolicyDoc(p, "bucket"); doc == "" {
			b.Fatal("bucketPolicyDoc() returned empty document")
		}
	}
}
