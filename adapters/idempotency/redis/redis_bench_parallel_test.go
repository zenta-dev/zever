package redis

import (
	"fmt"
	"testing"

	"github.com/zenta-dev/zever/core/idempotency"
)

// BenchmarkBeginParallel measures concurrent Begin reservation round trips
// over a shared miniredis instance.
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
