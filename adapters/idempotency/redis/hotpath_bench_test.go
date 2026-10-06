package redis

import (
	"fmt"
	"testing"
)

// BenchmarkForget measures the Delete round trip on a missing key.
func BenchmarkForget(b *testing.B) {
	s := benchStore(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		key := fmt.Sprintf("bench-%d", benchKeySeq.Add(1))

		if err := s.Forget(ctx, key); err != nil {
			b.Fatalf("Forget failed: %v", err)
		}
	}
}
