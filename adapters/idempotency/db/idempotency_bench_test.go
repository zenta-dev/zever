package db

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// benchKeySeq keeps Begin/Complete keys unique so each iteration exercises
// the reservation path rather than replay.
var benchKeySeq atomic.Int64

// benchStore opens a store over a private in-memory sqlite database.
func benchStore(b *testing.B) idempotency.Store {
	b.Helper()

	s, err := New(Options{})
	if err != nil {
		b.Fatalf("New: %v", err)
	}

	b.Cleanup(func() { _ = s.Close() })

	return s
}

// BenchmarkBegin measures the SetNX claim round trip on a missing key.
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

// BenchmarkBeginParallel measures concurrent claims over sqlite.
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

// BenchmarkComplete measures the get-then-set completion path on a missing key.
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
