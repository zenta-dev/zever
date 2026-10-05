package local

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/storage"
)

// newBenchLocal opens an unconfigured (legacy private) local adapter rooted in
// a temp dir, with a fixed secret so presigning is deterministic.
func newBenchLocal(b *testing.B) *localAdapter {
	b.Helper()

	s, err := New(storage.Options{
		URLBase:      "https://cdn.example.com",
		LocalOptions: storage.LocalOptions{Root: b.TempDir(), Secret: "bench-secret"},
	})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	a, ok := s.(*localAdapter)
	if !ok {
		b.Fatalf("New() type = %T, want *localAdapter", s)
	}

	return a
}

// BenchmarkPresignUpload measures minting a signed PUT URL: bucket/key
// validation, HMAC-SHA256 signing, and URL construction.
func BenchmarkPresignUpload(b *testing.B) {
	a := newBenchLocal(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.PresignUpload(ctx, "bucket", "dir/object.txt", "text/plain", time.Minute); err != nil {
			b.Fatalf("PresignUpload(): %v", err)
		}
	}
}

// BenchmarkPresignDownload measures minting a signed GET URL.
func BenchmarkPresignDownload(b *testing.B) {
	a := newBenchLocal(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.PresignDownload(ctx, "bucket", "dir/object.txt", time.Minute); err != nil {
			b.Fatalf("PresignDownload(): %v", err)
		}
	}
}

// BenchmarkHandlerPut measures the signed PUT round trip through the HTTP
// handler: signature verification, temp-file staging, and atomic rename.
func BenchmarkHandlerPut(b *testing.B) {
	a := newBenchLocal(b)
	ctx := b.Context()
	h := a.Handler()
	body := "bench-payload"

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		pu, err := a.PresignUpload(ctx, "bucket", "dir/object.txt", "text/plain", time.Minute)
		if err != nil {
			b.Fatalf("PresignUpload(): %v", err)
		}

		req := httptest.NewRequestWithContext(ctx, http.MethodPut, pu.URL, strings.NewReader(body))
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			b.Fatalf("PUT status = %d, want %d", rec.Code, http.StatusNoContent)
		}
	}
}

// BenchmarkHandlerGet measures the signed GET round trip: signature
// verification, symlink containment checks, and streaming the body back.
func BenchmarkHandlerGet(b *testing.B) {
	a := newBenchLocal(b)
	ctx := b.Context()
	h := a.Handler()

	pu, err := a.PresignUpload(ctx, "bucket", "dir/object.txt", "text/plain", time.Hour)
	if err != nil {
		b.Fatalf("PresignUpload(): %v", err)
	}

	putReq := httptest.NewRequestWithContext(ctx, http.MethodPut, pu.URL, strings.NewReader("bench-payload"))
	putRec := httptest.NewRecorder()
	h.ServeHTTP(putRec, putReq)

	if putRec.Code != http.StatusNoContent {
		b.Fatalf("setup PUT status = %d, want %d", putRec.Code, http.StatusNoContent)
	}

	pd, err := a.PresignDownload(ctx, "bucket", "dir/object.txt", time.Hour)
	if err != nil {
		b.Fatalf("PresignDownload(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, pd.URL, nil)
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			b.Fatalf("GET status = %d, want %d", rec.Code, http.StatusOK)
		}
	}
}

// BenchmarkEdgeExists measures a stat hit on an existing object.
func BenchmarkEdgeExists(b *testing.B) {
	a := newBenchLocal(b)
	ctx := b.Context()
	benchFile(b, a, "k")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.Exists(ctx, "bkt", "k"); err != nil {
			b.Fatalf("Exists(): %v", err)
		}
	}
}

// BenchmarkEdgeDelete measures removing an existing object.
func BenchmarkEdgeDelete(b *testing.B) {
	a := newBenchLocal(b)
	ctx := b.Context()
	benchFile(b, a, "k")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := a.Delete(ctx, "bkt", "k"); err != nil {
			b.Fatalf("Delete(): %v", err)
		}

		benchFile(b, a, "k")
	}
}

// BenchmarkEdgeMove measures renaming an object within a bucket.
func BenchmarkEdgeMove(b *testing.B) {
	a := newBenchLocal(b)
	ctx := b.Context()
	benchFile(b, a, "src")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := a.Move(ctx, "bkt", "src", "bkt", "dst"); err != nil {
			b.Fatalf("Move(): %v", err)
		}

		if err := a.Move(ctx, "bkt", "dst", "bkt", "src"); err != nil {
			b.Fatalf("Move back(): %v", err)
		}
	}
}
