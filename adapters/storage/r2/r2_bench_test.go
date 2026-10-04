package r2

import (
	"testing"
	"time"
)

// newBenchR2 opens an R2 adapter over the default R2 endpoint. Client
// construction is offline: static credentials and region, no network calls.
func newBenchR2(b *testing.B) *r2Adapter {
	b.Helper()

	s, err := New(validR2Options())
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	a, ok := s.(*r2Adapter)
	if !ok {
		b.Fatalf("New() type = %T, want *r2Adapter", s)
	}

	return a
}

// BenchmarkPresignUpload measures local SigV4 signing of a PUT URL for R2.
func BenchmarkPresignUpload(b *testing.B) {
	a := newBenchR2(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.PresignUpload(ctx, "bucket", "dir/object.txt", "text/plain", time.Minute); err != nil {
			b.Fatalf("PresignUpload(): %v", err)
		}
	}
}

// BenchmarkPresignDownload measures local SigV4 signing of a GET URL for R2.
func BenchmarkPresignDownload(b *testing.B) {
	a := newBenchR2(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.PresignDownload(ctx, "bucket", "dir/object.txt", time.Minute); err != nil {
			b.Fatalf("PresignDownload(): %v", err)
		}
	}
}

// BenchmarkStaticURL measures building the public unsigned URL for a bucket
// and key (no signing, no client).
func BenchmarkStaticURL(b *testing.B) {
	a := newBenchR2(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.StaticURL("bucket", "dir/object.txt"); err != nil {
			b.Fatalf("StaticURL(): %v", err)
		}
	}
}

// BenchmarkR2Endpoint measures account-ID validation and endpoint derivation,
// the pure path New takes before any client construction.
func BenchmarkR2Endpoint(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := r2Endpoint("testaccount123", ""); err != nil {
			b.Fatalf("r2Endpoint(): %v", err)
		}
	}
}
