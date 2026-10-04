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

	for i := 0; i < b.N; i++ {
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

	for i := 0; i < b.N; i++ {
		if err := a.Query(ctx, id, "state", &out); err != nil {
			b.Fatalf("Query(): %v", err)
		}
	}
}
