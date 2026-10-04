package memory

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// benchKeySeq keeps Begin/Complete keys unique so each iteration exercises
// the miss/reserve path rather than the in-progress or replay path.
var benchKeySeq atomic.Int64

// benchStore opens an in-memory store with defaults.
func benchStore(b *testing.B) *store {
	b.Helper()

	s, err := New(idempotency.Options{})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = s.Close() })

	ms, ok := s.(*store)
	if !ok {
		b.Fatalf("New type = %T, want *store", s)
	}

	return ms
}

// BenchmarkBegin measures validation, fingerprint check, and reservation.
func BenchmarkBegin(b *testing.B) {
	s := benchStore(b)
	ctx := b.Context()
	fp := []byte("fp")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

		if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
			b.Fatalf("Begin: %v", err)
		}
	}
}

// BenchmarkBeginParallel measures concurrent reservation of distinct keys.
func BenchmarkBeginParallel(b *testing.B) {
	s := benchStore(b)
	ctx := b.Context()
	fp := []byte("fp")

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

			if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
				b.Fatalf("Begin: %v", err)
			}
		}
	})
}

// BenchmarkComplete measures validation and result storage on a missing key.
func BenchmarkComplete(b *testing.B) {
	s := benchStore(b)
	ctx := b.Context()
	fp := []byte("fp")
	result := []byte("result")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

		if err := s.Complete(ctx, key, fp, result); err != nil {
			b.Fatalf("Complete: %v", err)
		}
	}
}
