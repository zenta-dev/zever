package memory

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// benchStore opens an in-memory store with a large bound so miss
// benchmarks measure the claim path, not eviction.
func benchStore(b *testing.B) idempotency.Store {
	b.Helper()

	s, err := New(idempotency.Options{MaxEntries: 1 << 20})
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}

	b.Cleanup(func() { _ = s.Close() })

	return s
}

// benchSeq numbers benchmark keys so each iteration claims a fresh record.
var benchSeq atomic.Int64

// BenchmarkNew measures constructing and closing the store, including the
// janitor goroutine start and join.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		s, err := New(idempotency.Options{})
		if err != nil {
			b.Fatalf("New failed: %v", err)
		}

		if err := s.Close(); err != nil {
			b.Fatalf("Close failed: %v", err)
		}
	}
}

// BenchmarkBeginMiss measures Begin on a fresh key: the reservation claim.
func BenchmarkBeginMiss(b *testing.B) {
	s := benchStore(b)

	ctx := b.Context()
	fp := []byte("fp")

	b.ReportAllocs()

	for b.Loop() {
		key := fmt.Sprintf("bench-%d", benchSeq.Add(1))

		if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
			b.Fatalf("Begin failed: %v", err)
		}
	}
}

// BenchmarkBeginReplay measures Begin against a completed record: the
// fingerprint check plus result clone.
func BenchmarkBeginReplay(b *testing.B) {
	s := benchStore(b)

	ctx := b.Context()
	fp := []byte("fp")
	key := "bench-replay"

	if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
		b.Fatalf("seed Begin failed: %v", err)
	}

	if err := s.Complete(ctx, key, fp, []byte("result")); err != nil {
		b.Fatalf("seed Complete failed: %v", err)
	}

	b.ReportAllocs()

	for b.Loop() {
		out, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp})
		if err != nil {
			b.Fatalf("Begin failed: %v", err)
		}

		if !out.Replay {
			b.Fatal("Begin Replay = false, want true")
		}
	}
}

// BenchmarkComplete measures Complete on a missing key: the upsert path.
func BenchmarkComplete(b *testing.B) {
	s := benchStore(b)

	ctx := b.Context()
	fp := []byte("fp")

	b.ReportAllocs()

	for b.Loop() {
		key := fmt.Sprintf("bench-%d", benchSeq.Add(1))

		if err := s.Complete(ctx, key, fp, []byte("result")); err != nil {
			b.Fatalf("Complete failed: %v", err)
		}
	}
}

// BenchmarkForget measures Forget on an absent key.
func BenchmarkForget(b *testing.B) {
	s := benchStore(b)

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		if err := s.Forget(ctx, "bench-missing"); err != nil {
			b.Fatalf("Forget failed: %v", err)
		}
	}
}

// BenchmarkBeginEviction measures Begin at capacity, forcing the eviction
// scan on every claim.
func BenchmarkBeginEviction(b *testing.B) {
	s, err := New(idempotency.Options{MaxEntries: 1})
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}

	b.Cleanup(func() { _ = s.Close() })

	ctx := b.Context()

	b.ReportAllocs()

	for b.Loop() {
		key := fmt.Sprintf("bench-%d", benchSeq.Add(1))

		if _, err := s.Begin(ctx, key, idempotency.BeginOptions{}); err != nil {
			b.Fatalf("Begin failed: %v", err)
		}
	}
}

// BenchmarkCheckFingerprint measures the fingerprint size guard.
func BenchmarkCheckFingerprint(b *testing.B) {
	fp := []byte("fingerprint")

	b.ReportAllocs()

	for b.Loop() {
		if err := checkFingerprint(fp); err != nil {
			b.Fatalf("checkFingerprint failed: %v", err)
		}
	}
}

// BenchmarkBeginParallel measures concurrent claims on distinct keys.
func BenchmarkBeginParallel(b *testing.B) {
	s := benchStore(b)

	ctx := b.Context()
	fp := []byte("fp")

	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			key := fmt.Sprintf("bench-%d", benchSeq.Add(1))

			if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
				b.Fatalf("Begin failed: %v", err)
			}
		}
	})
}
