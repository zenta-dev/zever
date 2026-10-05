package s3

import (
	"net/http"
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

	for b.Loop() {
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

	for b.Loop() {
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

	for b.Loop() {
		if doc := bucketPolicyDoc(p, "bucket"); doc == "" {
			b.Fatal("bucketPolicyDoc() returned empty document")
		}
	}
}

// BenchmarkEdgeSyncPolicyNoDrift measures the get+merge+compare path when the
// live policy already matches the generated document byte-for-byte.
func BenchmarkEdgeSyncPolicyNoDrift(b *testing.B) {
	current := bucketPolicyDoc(*publicReadPolicyConfig().Default, "b")
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		return xmlResponse(r, 200, current), nil
	}}
	a := newBenchStubAdapter(b, publicReadPolicyConfig(), "require", tr)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := a.SyncPolicy(ctx, "b"); err != nil {
			b.Fatalf("SyncPolicy(): %v", err)
		}
	}
}

// BenchmarkEdgeSyncPolicyDrift measures the drift path: get, merge, and put.
func BenchmarkEdgeSyncPolicyDrift(b *testing.B) {
	current := `{"Version":"2012-10-17","Statement":[{"Sid":"zever-read","Effect":"Deny",` +
		`"Principal":{"AWS":"*"},"Action":"s3:GetObject","Resource":["arn:aws:s3:::b/*"]}]}`
	tr := &stubTransport{do: func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodPut {
			return xmlResponse(r, 200, ""), nil
		}

		return xmlResponse(r, 200, current), nil
	}}
	a := newBenchStubAdapter(b, publicReadPolicyConfig(), "require", tr)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := a.SyncPolicy(ctx, "b"); err != nil {
			b.Fatalf("SyncPolicy(): %v", err)
		}
	}
}
