package kvstore

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
)

func newBenchStore(b *testing.B) *Store {
	b.Helper()

	path := filepath.Join(b.TempDir(), "kv.db")

	db, err := dbsqlite.New(coredb.Options{Path: path})
	if err != nil {
		b.Fatalf("sqlite New failed: %v", err)
	}

	b.Cleanup(func() { _ = db.Close(context.Background()) })

	s, err := New(db, "")
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}

	return s
}

const benchKeyCount = 256

func benchKeys() []string {
	keys := make([]string, benchKeyCount)
	for i := range keys {
		keys[i] = "k" + strconv.Itoa(i)
	}

	return keys
}

// BenchmarkSet measures the upsert write path.
func BenchmarkSet(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()
	keys := benchKeys()
	value := []byte("value")

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		if err := s.Set(ctx, keys[i%len(keys)], value, time.Hour); err != nil {
			b.Fatalf("Set() error = %v", err)
		}
	}
}

// BenchmarkGet measures the read path on a warm store.
func BenchmarkGet(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()
	keys := benchKeys()

	for _, k := range keys {
		if err := s.Set(ctx, k, []byte("value"), time.Hour); err != nil {
			b.Fatalf("Set() setup error = %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		if _, ok, err := s.Get(ctx, keys[i%len(keys)]); err != nil || !ok {
			b.Fatalf("Get() = (%v, %v)", ok, err)
		}
	}
}

// BenchmarkAddDelta measures the transactional read-modify-write counter path.
func BenchmarkAddDelta(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := s.AddDelta(ctx, "counter", 1); err != nil {
			b.Fatalf("AddDelta() error = %v", err)
		}
	}
}

// BenchmarkGetWithExpiry measures the read path returning the expiry timestamp.
func BenchmarkGetWithExpiry(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()
	keys := benchKeys()

	for _, k := range keys {
		if err := s.Set(ctx, k, []byte("value"), time.Hour); err != nil {
			b.Fatalf("Set() setup error = %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		if _, _, ok, err := s.GetWithExpiry(ctx, keys[i%len(keys)]); err != nil || !ok {
			b.Fatalf("GetWithExpiry() = (%v, %v)", ok, err)
		}

		i++
	}
}

// BenchmarkSetNX measures the conditional insert path on an existing key.
func BenchmarkSetNX(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()
	keys := benchKeys()

	for _, k := range keys {
		if err := s.Set(ctx, k, []byte("value"), time.Hour); err != nil {
			b.Fatalf("Set() setup error = %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		if _, err := s.SetNX(ctx, keys[i%len(keys)], []byte("other"), time.Hour); err != nil {
			b.Fatalf("SetNX() error = %v", err)
		}

		i++
	}
}

// BenchmarkDelete measures the delete path on an existing key.
func BenchmarkDelete(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()
	keys := benchKeys()

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		key := keys[i%len(keys)]

		if err := s.Set(ctx, key, []byte("value"), time.Hour); err != nil {
			b.Fatalf("Set() setup error = %v", err)
		}

		if err := s.Delete(ctx, key); err != nil {
			b.Fatalf("Delete() error = %v", err)
		}

		i++
	}
}

// BenchmarkSweepExpired measures the lazy-expiry sweep path.
func BenchmarkSweepExpired(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := s.SweepExpired(ctx); err != nil {
			b.Fatalf("SweepExpired() error = %v", err)
		}
	}
}

// BenchmarkGetParallel measures concurrent reads across a warm store.
func BenchmarkGetParallel(b *testing.B) {
	s := newBenchStore(b)
	ctx := b.Context()
	keys := benchKeys()

	for _, k := range keys {
		if err := s.Set(ctx, k, []byte("value"), time.Hour); err != nil {
			b.Fatalf("Set() setup error = %v", err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			_, _, _ = s.Get(ctx, keys[i%len(keys)])
			i++
		}
	})
}
