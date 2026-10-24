package memory

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// benchJTISeq numbers revoked jtis so each Revoke iteration stores a fresh
// entry instead of hitting the idempotent-duplicate fast path.
var benchJTISeq atomic.Int64

// benchUntil is the fixed revocation horizon shared by every benchmark. It
// is read once at setup; the per-call clock read inside Revoke/IsRevoked is
// part of the code under test.
var benchUntil = time.Now().Add(time.Hour)

// newBenchStore opens a store for benchmarking and registers its cleanup.
func newBenchStore(b *testing.B) *store {
	b.Helper()

	s, err := New(Options{})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	st, ok := s.(*store)
	if !ok {
		b.Fatalf("New() returned %T, want *store", s)
	}

	b.Cleanup(func() {
		if err := s.Close(); err != nil {
			b.Errorf("Close() error = %v", err)
		}
	})

	return st
}

// BenchmarkRevoke measures storing a fresh revocation (map insert plus heap
// push).
func BenchmarkRevoke(b *testing.B) {
	s := newBenchStore(b)

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		jti := "bench-" + strconv.FormatInt(benchJTISeq.Add(1), 10)

		if err := s.Revoke(ctx, jti, benchUntil); err != nil {
			b.Fatalf("Revoke() error = %v", err)
		}
	}
}

// BenchmarkRevokeDuplicate measures the idempotent re-revoke fast path for
// an already-revoked jti.
func BenchmarkRevokeDuplicate(b *testing.B) {
	s := newBenchStore(b)

	ctx := b.Context()

	if err := s.Revoke(ctx, "bench-dup", benchUntil); err != nil {
		b.Fatalf("seed Revoke() error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Revoke(ctx, "bench-dup", benchUntil); err != nil {
			b.Fatalf("Revoke() error = %v", err)
		}
	}
}

// BenchmarkIsRevokedHit measures the lookup for a currently-revoked jti.
// The store is pre-populated with 1024 entries cycled uniformly.
func BenchmarkIsRevokedHit(b *testing.B) {
	s := newBenchStore(b)

	ctx := b.Context()

	const seeded = 1024

	for i := 0; i < seeded; i++ {
		if err := s.Revoke(ctx, "bench-hit-"+strconv.Itoa(i), benchUntil); err != nil {
			b.Fatalf("seed Revoke() error = %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ok, err := s.IsRevoked(ctx, "bench-hit-"+strconv.Itoa(i%seeded))
		if err != nil {
			b.Fatalf("IsRevoked() error = %v", err)
		}

		if !ok {
			b.Fatal("IsRevoked() = false, want true")
		}
	}
}

// BenchmarkIsRevokedMiss measures the lookup for a never-revoked jti.
func BenchmarkIsRevokedMiss(b *testing.B) {
	s := newBenchStore(b)

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		jti := "bench-miss-" + strconv.FormatInt(benchJTISeq.Add(1), 10)

		ok, err := s.IsRevoked(ctx, jti)
		if err != nil {
			b.Fatalf("IsRevoked() error = %v", err)
		}

		if ok {
			b.Fatalf("IsRevoked(%q) = true, want false", jti)
		}
	}
}

// BenchmarkRevokeIsRevoked measures the revoke-then-check round trip on a
// fresh jti per iteration.
func BenchmarkRevokeIsRevoked(b *testing.B) {
	s := newBenchStore(b)

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		jti := "bench-rt-" + strconv.FormatInt(benchJTISeq.Add(1), 10)

		if err := s.Revoke(ctx, jti, benchUntil); err != nil {
			b.Fatalf("Revoke() error = %v", err)
		}

		ok, err := s.IsRevoked(ctx, jti)
		if err != nil {
			b.Fatalf("IsRevoked() error = %v", err)
		}

		if !ok {
			b.Fatal("IsRevoked() = false, want true")
		}
	}
}

// BenchmarkRevokeParallel measures concurrent revocations on distinct jtis.
func BenchmarkRevokeParallel(b *testing.B) {
	s := newBenchStore(b)

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			jti := "bench-par-" + strconv.FormatInt(benchJTISeq.Add(1), 10)

			if err := s.Revoke(ctx, jti, benchUntil); err != nil {
				b.Fatalf("Revoke() error = %v", err)
			}
		}
	})
}

// BenchmarkNewClose measures constructing and closing the store, including
// pruner goroutine startup and shutdown.
func BenchmarkNewClose(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		s, err := New(Options{MaxEntries: 128})
		if err != nil {
			b.Fatalf("New() error = %v", err)
		}

		if err := s.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}
