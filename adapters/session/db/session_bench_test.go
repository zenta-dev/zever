package db

import (
	"path/filepath"
	"testing"
	"time"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/session"
)

// newBenchStore opens a file-backed sqlite store for benchmarks. A file (not
// ":memory:") keeps each benchmark isolated from process-global shared cache.
func newBenchStore(b *testing.B) session.Store {
	b.Helper()

	s, err := New(Options{Options: coredb.Options{Path: filepath.Join(b.TempDir(), "bench.db")}})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	b.Cleanup(func() { _ = s.Close() })

	return s
}

// BenchmarkCreate measures minting and persisting a fresh session row.
func BenchmarkCreate(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := s.Create(ctx, time.Hour); err != nil {
			b.Fatalf("Create(): %v", err)
		}
	}
}

// BenchmarkGet measures a store hit: one indexed kv read plus JSON decode.
func BenchmarkGet(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	sess, err := s.Create(ctx, time.Hour)
	if err != nil {
		b.Fatalf("Create(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := s.Get(ctx, sess.ID); err != nil {
			b.Fatalf("Get(): %v", err)
		}
	}
}

// BenchmarkSave measures the update path: read-merge-write of an existing row.
func BenchmarkSave(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	sess, err := s.Create(ctx, time.Hour)
	if err != nil {
		b.Fatalf("Create(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Save(ctx, sess); err != nil {
			b.Fatalf("Save(): %v", err)
		}
	}
}

// BenchmarkDelete measures removing an existing session row. Delete is
// idempotent, so the same ID is reused.
func BenchmarkDelete(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	sess, err := s.Create(ctx, time.Hour)
	if err != nil {
		b.Fatalf("Create(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Delete(ctx, sess.ID); err != nil {
			b.Fatalf("Delete(): %v", err)
		}
	}
}

// BenchmarkGetParallel measures read throughput on one hot session under
// concurrent access, exercising the shared connection pool.
func BenchmarkGetParallel(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	sess, err := s.Create(ctx, time.Hour)
	if err != nil {
		b.Fatalf("Create(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := s.Get(ctx, sess.ID); err != nil {
				b.Errorf("Get(): %v", err)
				return
			}
		}
	})
}
