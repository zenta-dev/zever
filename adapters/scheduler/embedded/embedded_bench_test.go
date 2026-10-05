package embedded

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/scheduler"
)

func benchRegister(b *testing.B, name string) {
	b.Helper()
	if err := job.Register(name, func(context.Context, string) error { return nil }); err != nil {
		var dup job.DuplicateJobError
		if !errors.As(err, &dup) {
			b.Fatalf("Register: %v", err)
		}
	}
}

func benchScheduler(b *testing.B) scheduler.Scheduler {
	b.Helper()
	s, err := New(scheduler.Options{Dispatcher: &job.Dispatcher{Q: &stubQueue{}}})
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	return s
}

// BenchmarkSchedule measures spec parsing plus cron registration (and Remove)
// per iteration.
func BenchmarkSchedule(b *testing.B) {
	benchRegister(b, "bench-embedded-schedule")
	s := benchScheduler(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		id, err := s.Schedule(ctx, "0 * * * *", "bench-embedded-schedule", nil)
		if err != nil {
			b.Fatalf("Schedule: %v", err)
		}
		if err := s.Remove(id); err != nil {
			b.Fatalf("Remove: %v", err)
		}
	}
}

// BenchmarkEntries measures snapshotting the live entry set.
func BenchmarkEntries(b *testing.B) {
	benchRegister(b, "bench-embedded-entries")
	s := benchScheduler(b)
	ctx := b.Context()
	for i := 0; i < 16; i++ {
		if _, err := s.Schedule(ctx, "0 * * * *", "bench-embedded-entries", nil); err != nil {
			b.Fatalf("Schedule: %v", err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if got := s.Entries(); len(got) != 16 {
			b.Fatalf("Entries() len = %d, want 16", len(got))
		}
	}
}
