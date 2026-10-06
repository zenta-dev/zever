package postgres

import (
	"testing"
	"time"
)

// BenchmarkClaim measures the lease-transfer UPDATE that every tick runs
// before dispatch.
func BenchmarkClaim(b *testing.B) {
	benchRegister(b, "bench-pg-claim")

	d := benchDriver(b)
	ctx := b.Context()

	id, err := d.Schedule(ctx, "0 * * * *", "bench-pg-claim", nil)
	if err != nil {
		b.Fatalf("Schedule: %v", err)
	}

	slot := d.slots[id]
	now := time.Now().UTC()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := d.claim(ctx, slot, now); err != nil {
			b.Fatalf("claim: %v", err)
		}
	}
}
