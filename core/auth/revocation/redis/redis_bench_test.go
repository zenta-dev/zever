package redis

import (
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	redisopt "github.com/zenta-dev/zever/shared/redisopt"
)

// benchJTISeq numbers jtis so each iteration exercises a fresh key instead
// of re-hitting the same entry.
var benchJTISeq atomic.Int64

// benchUntil is the fixed revocation horizon shared by every benchmark. It
// is read once at setup; the per-call clock read inside Revoke is part of
// the code under test.
var benchUntil = time.Now().Add(time.Hour)

// newBenchStore opens a store over a throwaway in-process miniredis instance
// and registers cleanup for both the server and the store. No external
// network is involved: miniredis serves on loopback only.
func newBenchStore(b *testing.B) *store {
	b.Helper()

	srv, err := miniredis.Run()
	if err != nil {
		b.Fatalf("miniredis Run() error = %v", err)
	}

	b.Cleanup(srv.Close)

	st, err := New(Options{ConnectOptions: redisopt.ConnectOptions{Addr: srv.Addr()}})
	if err != nil {
		b.Fatalf("New() error = %v", err)
	}

	b.Cleanup(func() {
		if err := st.Close(); err != nil {
			b.Errorf("Close() error = %v", err)
		}
	})

	si, ok := st.(*store)
	if !ok {
		b.Fatalf("New() returned %T, want *store", st)
	}

	return si
}

// BenchmarkRevoke measures storing a fresh revocation (one SET with TTL
// round trip against the backing Redis).
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

// BenchmarkIsRevokedHit measures the lookup for a currently-revoked jti
// (one EXISTS round trip). The store is pre-populated with 1024 entries
// cycled uniformly.
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

	i := 0

	for b.Loop() {
		ok, err := s.IsRevoked(ctx, "bench-hit-"+strconv.Itoa(i%seeded))
		if err != nil {
			b.Fatalf("IsRevoked() error = %v", err)
		}

		if !ok {
			b.Fatal("IsRevoked() = false, want true")
		}

		i++
	}
}

// BenchmarkIsRevokedMiss measures the lookup for a never-revoked jti (one
// EXISTS round trip returning zero).
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
