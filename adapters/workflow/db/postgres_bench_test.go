package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
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

	for b.Loop() {
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

	for b.Loop() {
		if err := d.Query(ctx, id, "state", &out); err != nil {
			b.Fatalf("Query(): %v", err)
		}
	}
}

// BenchmarkSignal measures recording a signal payload on a running run: one
// row load plus the state-guarded payload update.
func BenchmarkSignal(b *testing.B) {
	d := newBenchDriver(b)
	benchRunningRun(b, d, "sig-bench")

	ctx := b.Context()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := d.Signal(ctx, "sig-bench", "advance", "value"); err != nil {
			b.Fatalf("Signal(): %v", err)
		}
	}
}

// BenchmarkCancel measures cancelling a completed run: one row load followed
// by the completed-run guard.
func BenchmarkCancel(b *testing.B) {
	d := newBenchDriver(b)

	ctx := b.Context()

	id, err := d.Start(ctx, "bench", "input", "cancel-bench")
	if err != nil {
		b.Fatalf("Start(): %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if err := d.Cancel(ctx, id); !errors.Is(err, workflow.ErrRunCompleted) {
			b.Fatalf("Cancel(): %v, want ErrRunCompleted", err)
		}
	}
}

// benchRunningRun starts a run whose step blocks until cleanup, leaving the
// row running for the whole benchmark. The release is registered with
// b.Cleanup so it runs before the driver is closed. It uses a background
// context because b.Context is cancelled before cleanup functions run.
func benchRunningRun(b *testing.B, d *driver, id string) {
	b.Helper()

	started := make(chan struct{})
	releaseCh := make(chan struct{})

	d.RegisterStep("bench-block", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-releaseCh

		return "done", nil
	})

	done := make(chan error, 1)

	go func() {
		_, err := d.Start(context.Background(), "bench-block", "input", id)
		done <- err
	}()

	<-started

	b.Cleanup(func() {
		close(releaseCh)

		if err := <-done; err != nil {
			b.Errorf("Start(%s) error = %v", id, err)
		}
	})
}
