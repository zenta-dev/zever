package memory_test

import (
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/session"
)

// newBenchStore opens a memory session store for benchmarks.
func newBenchStore(b *testing.B) session.Store {
	b.Helper()

	st, err := memory.New(session.Options{TTL: time.Hour})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	b.Cleanup(func() { _ = st.Close() })

	return st
}

// BenchmarkCreate measures allocating and storing a fresh session.
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

// BenchmarkGet measures a store hit: map lookup plus a defensive clone.
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

// BenchmarkSave measures the update path for an existing session.
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

// BenchmarkGetParallel measures read throughput on one hot session under
// concurrent access, exercising the RWMutex read path.
func BenchmarkGetParallel(b *testing.B) {
	st := newBenchStore(b)
	ctx := b.Context()

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		b.Fatalf("Create(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := st.Get(ctx, s.ID); err != nil {
				b.Errorf("Get(): %v", err)
				return
			}
		}
	})
}

// BenchmarkCreateParallel measures write throughput creating distinct sessions
// from many goroutines, exercising the write lock.
func BenchmarkCreateParallel(b *testing.B) {
	st := newBenchStore(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := st.Create(ctx, time.Hour); err != nil {
				b.Errorf("Create(): %v", err)
				return
			}
		}
	})
}
