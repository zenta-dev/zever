package embedded

import "testing"

// BenchmarkRemove measures unregistering a live cron entry, the delete side
// of the Schedule round trip.
func BenchmarkRemove(b *testing.B) {
	benchRegister(b, "bench-embedded-remove")

	s := benchScheduler(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()

		id, err := s.Schedule(ctx, "0 * * * *", "bench-embedded-remove", nil)
		if err != nil {
			b.Fatalf("Schedule: %v", err)
		}

		b.StartTimer()

		if err := s.Remove(id); err != nil {
			b.Fatalf("Remove: %v", err)
		}
	}
}
