package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/core/workflow"
)

// newSagaBenchDriver opens a file-backed sqlite engine with a two-step saga.
func newSagaBenchDriver(b *testing.B) *driver {
	b.Helper()

	w, err := New(Options{Path: filepath.Join(b.TempDir(), "saga-bench.db"), Owner: "bench-owner"})
	if err != nil {
		b.Fatalf("New error = %v", err)
	}

	d, ok := w.(*driver)
	if !ok {
		b.Fatalf("New returned %T, want *driver", w)
	}

	d.RegisterSaga("bench", []workflow.SagaStep{
		{Name: "a", Execute: func(_ context.Context, in any) (any, error) { return in, nil }},
		{Name: "b", Execute: func(_ context.Context, in any) (any, error) { return in, nil }},
	})

	b.Cleanup(func() { _ = w.Close() })

	return d
}

// BenchmarkRunSaga measures saga execution: input marshal, run-row insert,
// step execution and progress persistence.
func BenchmarkRunSaga(b *testing.B) {
	d := newSagaBenchDriver(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := d.RunSaga(ctx, "bench", "input", ""); err != nil {
			b.Fatalf("RunSaga error = %v", err)
		}
	}
}

// BenchmarkSagaStatus measures saga status lookup.
func BenchmarkSagaStatus(b *testing.B) {
	d := newSagaBenchDriver(b)
	ctx := b.Context()

	id, err := d.RunSaga(ctx, "bench", "input", "")
	if err != nil {
		b.Fatalf("RunSaga error = %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := d.SagaStatus(ctx, id); err != nil {
			b.Fatalf("SagaStatus error = %v", err)
		}
	}
}
