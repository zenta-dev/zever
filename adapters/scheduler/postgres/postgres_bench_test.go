package postgres

import (
	"errors"
	"path/filepath"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/job"
)

func benchRegister(b *testing.B, name string) {
	b.Helper()
	if err := job.Register(name, echoJob); err != nil {
		var dup *job.DuplicateJobError
		if !errors.As(err, &dup) {
			b.Fatalf("Register: %v", err)
		}
	}
}

func benchDriver(b *testing.B) *driver {
	b.Helper()
	opts := Options{
		Owner:       "owner-bench",
		PoolOptions: coredb.Options{Path: filepath.Join(b.TempDir(), "scheduler.db")},
	}
	opts.Dispatcher = &job.Dispatcher{Q: newStubQueue()}
	s, err := New(opts)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	d, ok := s.(*driver)
	if !ok {
		b.Fatalf("New returned %T, want *driver", s)
	}
	b.Cleanup(func() { _ = d.Close() })
	return d
}

// BenchmarkSchedule measures the DB slot insert/delete plus local cron
// registration round trip.
func BenchmarkSchedule(b *testing.B) {
	benchRegister(b, "bench-pg-schedule")
	d := benchDriver(b)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, err := d.Schedule(ctx, "0 * * * *", "bench-pg-schedule", nil)
		if err != nil {
			b.Fatalf("Schedule: %v", err)
		}
		if err := d.Remove(id); err != nil {
			b.Fatalf("Remove: %v", err)
		}
	}
}

// BenchmarkLoad measures a single slot-row fetch.
func BenchmarkLoad(b *testing.B) {
	benchRegister(b, "bench-pg-load")
	d := benchDriver(b)
	ctx := b.Context()
	id, err := d.Schedule(ctx, "0 * * * *", "bench-pg-load", nil)
	if err != nil {
		b.Fatalf("Schedule: %v", err)
	}
	slot := d.slots[id]
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, ok, err := d.load(ctx, slot); err != nil || !ok {
			b.Fatalf("load = ok=%v err=%v", ok, err)
		}
	}
}

// BenchmarkEntries measures snapshotting the live entry set.
func BenchmarkEntries(b *testing.B) {
	benchRegister(b, "bench-pg-entries")
	d := benchDriver(b)
	ctx := b.Context()
	for i := 0; i < 16; i++ {
		if _, err := d.Schedule(ctx, "0 * * * *", "bench-pg-entries", nil); err != nil {
			b.Fatalf("Schedule: %v", err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if got := d.Entries(); len(got) != 16 {
			b.Fatalf("Entries() len = %d, want 16", len(got))
		}
	}
}
