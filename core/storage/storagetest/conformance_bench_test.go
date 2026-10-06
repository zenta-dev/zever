package storagetest

import (
	"testing"

	"github.com/zenta-dev/zever/core/storage"
)

// BenchmarkConformanceChecks measures the full kit over a correct backend.
func BenchmarkConformanceChecks(b *testing.B) {
	s := stubOKStorage{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r := &stubReporter{}
		conformancePresignUpload(r, s)
		conformancePresignDownload(r, s)
		conformanceExists(r, s)
		conformanceDelete(r, s)
		conformanceMove(r, s)
		conformanceClose(r, s)

		if got := r.joined(); got != "" {
			b.Fatalf("correct backend produced reports:\n%s", got)
		}
	}
}

// BenchmarkConformancePresignUpload measures the upload checks alone.
func BenchmarkConformancePresignUpload(b *testing.B) {
	var s storage.Storage = stubOKStorage{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r := &stubReporter{}
		conformancePresignUpload(r, s)

		if got := r.joined(); got != "" {
			b.Fatalf("correct backend produced reports:\n%s", got)
		}
	}
}
