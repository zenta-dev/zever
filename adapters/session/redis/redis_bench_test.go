package redis_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	sredis "github.com/zenta-dev/zever/adapters/session/redis"
	"github.com/zenta-dev/zever/core/session"
)

// newBenchStore opens a store over a throwaway miniredis instance.
func newBenchStore(b *testing.B) session.Store {
	b.Helper()

	srv := miniredis.RunT(b)

	st, err := sredis.New(session.Options{Redis: session.RedisOptions{Addr: srv.Addr()}})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	b.Cleanup(func() { _ = st.Close() })

	return st
}

// BenchmarkCreate measures the SET EX write for a fresh session.
func BenchmarkCreate(b *testing.B) {
	st := newBenchStore(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := st.Create(ctx, time.Hour); err != nil {
			b.Fatalf("Create(): %v", err)
		}
	}
}

// BenchmarkGet measures the GET + JSON decode read path.
func BenchmarkGet(b *testing.B) {
	st := newBenchStore(b)
	ctx := b.Context()

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		b.Fatalf("Create(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := st.Get(ctx, s.ID); err != nil {
			b.Fatalf("Get(): %v", err)
		}
	}
}

// BenchmarkSave measures the WATCH/MULTI/EXEC update path on an existing key.
func BenchmarkSave(b *testing.B) {
	st := newBenchStore(b)
	ctx := b.Context()

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		b.Fatalf("Create(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := st.Save(ctx, s); err != nil {
			b.Fatalf("Save(): %v", err)
		}
	}
}

// BenchmarkDelete measures the DEL round trip for an existing session. Delete
// is idempotent, so the same ID is reused.
func BenchmarkDelete(b *testing.B) {
	st := newBenchStore(b)
	ctx := b.Context()

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		b.Fatalf("Create(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := st.Delete(ctx, s.ID); err != nil {
			b.Fatalf("Delete(): %v", err)
		}
	}
}
