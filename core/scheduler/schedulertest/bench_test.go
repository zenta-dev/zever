package schedulertest

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/job"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
)

// BenchmarkStubQueuePush measures stub queue push throughput.
func BenchmarkStubQueuePush(b *testing.B) {
	s := newStubQueue()
	ctx := b.Context()
	payload := queue.Payload("bench")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Push(ctx, "bench", payload, nil); err != nil {
			b.Fatalf("Push error = %v", err)
		}
	}
}

// BenchmarkStubQueuePop measures stub queue pop throughput.
func BenchmarkStubQueuePop(b *testing.B) {
	s := newStubQueue()
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := s.Push(ctx, "bench", queue.Payload("bench"), nil); err != nil {
			b.Fatalf("Push error = %v", err)
		}

		if _, err := s.Pop(ctx, "bench"); err != nil {
			b.Fatalf("Pop error = %v", err)
		}
	}
}

// healthyBenchScheduler returns a stub satisfying the schedule-entries
// check so benchmarks measure the assertion path.
func healthyBenchScheduler() *stubScheduler {
	stub := healthyStubScheduler()
	stub.name = "bench"

	return stub
}

// benchJobName registers a process-unique no-op job for benchmarks.
func benchJobName(b *testing.B) string {
	b.Helper()

	name := "bench-job-schedule"

	_ = job.Register(name, func(_ context.Context, _ string) error { return nil })

	return name
}

// BenchmarkCheckScheduleEntries measures the schedule-entries hot path.
func BenchmarkCheckScheduleEntries(b *testing.B) {
	ctx := b.Context()
	name := benchJobName(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkScheduleEntries(ctx, healthyBenchScheduler(), name); err != nil {
			b.Fatalf("checkScheduleEntries() err = %v", err)
		}
	}
}

// BenchmarkCheckStartStop measures the start-stop assertion hot path.
func BenchmarkCheckStartStop(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkStartStop(healthyBenchScheduler()); err != nil {
			b.Fatalf("checkStartStop() err = %v", err)
		}
	}
}

// BenchmarkCheckRemove measures the remove assertion hot path.
func BenchmarkCheckRemove(b *testing.B) {
	ctx := b.Context()
	name := benchJobName(b)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := checkRemove(ctx, healthyBenchScheduler(), name); err != nil {
			b.Fatalf("checkRemove() err = %v", err)
		}
	}
}

var _ scheduler.Scheduler = healthyStubScheduler()
