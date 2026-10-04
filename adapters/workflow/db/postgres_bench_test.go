package db

import (
	"context"
	"path/filepath"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
)

// newBenchDriver opens a file-backed sqlite workflow engine with an identity
// step and a fixed owner.
func newBenchDriver(b *testing.B) *driver {
	b.Helper()

	w, err := New(Options{
		Options: coredb.Options{Path: filepath.Join(b.TempDir(), "workflow.db")},
		Owner:   "bench-owner",
	})
	if err != nil {
		b.Fatalf("New(): %v", err)
	}

	d, ok := w.(*driver)
	if !ok {
		b.Fatalf("New() type = %T, want *driver", w)
	}

	d.RegisterStep("bench", func(_ context.Context, in any) (any, error) { return in, nil })

	b.Cleanup(func() { _ = d.Close() })

	return d
}

// BenchmarkStart measures starting a run: ID allocation, a running-row insert
// holding the lease, synchronous step execution, and the completion update.
func BenchmarkStart(b *testing.B) {
	d := newBenchDriver(b)
	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := d.Start(ctx, "bench", "input", ""); err != nil {
			b.Fatalf("Start(): %v", err)
		}
	}
}

// BenchmarkQuery measures reading a completed run: one row load plus a JSON
// decode of the stored payload.
func BenchmarkQuery(b *testing.B) {
	d := newBenchDriver(b)
	ctx := b.Context()

	id, err := d.Start(ctx, "bench", "input", "")
	if err != nil {
		b.Fatalf("Start(): %v", err)
	}

	var out string

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if err := d.Query(ctx, id, "state", &out); err != nil {
			b.Fatalf("Query(): %v", err)
		}
	}
}
