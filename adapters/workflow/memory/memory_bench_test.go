package memory

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

// newBenchAdapter opens an in-memory workflow engine with an identity step.
func newBenchAdapter(b *testing.B) *Adapter {
	b.Helper()

	w, err := New(workflow.Options{})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	a, ok := w.(*Adapter)
	if !ok {
		b.Fatalf("New() type = %T, want *Adapter", w)
	}

	a.RegisterStep("bench", func(_ context.Context, in any) (any, error) { return in, nil })

	b.Cleanup(func() { _ = a.Close() })

	return a
}

// BenchmarkStart measures starting a run: ID allocation, step lookup, step
// execution, and state publication.
func BenchmarkStart(b *testing.B) {
	a := newBenchAdapter(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := a.Start(ctx, "bench", "input", ""); err != nil {
			b.Fatalf("Start(): %v", err)
		}
	}
}

// BenchmarkQuery measures reading a completed run's state: map lookup plus a
// JSON marshal/unmarshal round trip.
func BenchmarkQuery(b *testing.B) {
	a := newBenchAdapter(b)
	ctx := b.Context()

	id, err := a.Start(ctx, "bench", "input", "")
	if err != nil {
		b.Fatalf("Start(): %v", err)
	}

	var out string

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := a.Query(ctx, id, "state", &out); err != nil {
			b.Fatalf("Query(): %v", err)
		}
	}
}

// BenchmarkSignal measures buffering a signal on a running run. A running run
// is seeded directly so the success path (lock, map lookup, append) can be
// measured without hitting the 100-entry pending cap.
func BenchmarkSignal(b *testing.B) {
	a := newBenchAdapter(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		a.mu.Lock()
		a.runs["bench-run"] = &run{running: true}
		a.mu.Unlock()

		if err := a.Signal(ctx, "bench-run", "advance", "value"); err != nil {
			b.Fatalf("Signal(): %v", err)
		}
	}
}

// BenchmarkCancel measures cancelling a running run. A running run is seeded
// directly each iteration so the success path (lock, map lookup, delete) can
// be measured repeatably.
func BenchmarkCancel(b *testing.B) {
	a := newBenchAdapter(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		a.mu.Lock()
		a.runs["bench-run"] = &run{running: true}
		a.mu.Unlock()

		if err := a.Cancel(ctx, "bench-run"); err != nil {
			b.Fatalf("Cancel(): %v", err)
		}
	}
}
