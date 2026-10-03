package redis

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/zenta-dev/zever/core/idempotency"
)

// keySeq keeps generated keys unique within one benchmark's shared miniredis
// instance, so every iteration exercises the reservation-miss path (SET NX
// succeeds) rather than the in-progress path.
var benchKeySeq atomic.Int64

// benchStore opens a store over a throwaway miniredis instance for benchmarks.
func benchStore(b *testing.B) *store {
	b.Helper()

	s, err := miniredis.Run()
	if err != nil {
		b.Fatalf("miniredis Run failed: %v", err)
	}

	b.Cleanup(func() { s.Close() })

	st, err := New(idempotency.Options{Redis: idempotency.RedisOptions{Addr: s.Addr()}})
	if err != nil {
		b.Fatalf("New failed: %v", err)
	}

	b.Cleanup(func() { _ = st.Close() })

	ds, ok := st.(*store)
	if !ok {
		b.Fatalf("New store type = %T, want *store", st)
	}

	return ds
}

// BenchmarkBegin measures the Begin reservation round trip: one Lua script
// that SET NX on miss or classifies the existing record.
func BenchmarkBegin(b *testing.B) {
	s := benchStore(b)
	ctx := b.Context()
	fp := []byte("fp")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

		if _, err := s.Begin(ctx, key, idempotency.BeginOptions{Fingerprint: fp}); err != nil {
			b.Fatalf("Begin failed: %v", err)
		}
	}
}

// BenchmarkComplete measures the Complete round trip on a missing key (the
// upsert path): one Lua script that GETs, then SETs under the store TTL.
func BenchmarkComplete(b *testing.B) {
	s := benchStore(b)
	ctx := b.Context()
	fp := []byte("fp")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

		if err := s.Complete(ctx, key, fp, []byte("result")); err != nil {
			b.Fatalf("Complete failed: %v", err)
		}
	}
}
