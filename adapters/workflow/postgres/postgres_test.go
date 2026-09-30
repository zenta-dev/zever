package postgres

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/orm"
)

func echoStep(_ context.Context, input any) (any, error) {
	return input, nil
}

func mustNew(t *testing.T, opts Options) *driver {
	t.Helper()

	if opts.Path == "" && opts.DSN == "" {
		opts.Path = ":memory:"
	}

	if opts.Owner == "" {
		opts.Owner = "owner-test"
	}

	w, err := New(opts)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	d, ok := w.(*driver)
	if !ok {
		t.Fatalf("New returned %T, want *driver", w)
	}

	t.Cleanup(func() { _ = w.Close() })

	return d
}

func TestCreateRunToComplete(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-1"})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "greet", "hello", "run-1")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if id != workflow.RunID("run-1") {
		t.Fatalf("RunID = %q, want %q", id, "run-1")
	}

	var out string
	if err := d.Query(ctx, id, "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if out != "hello" {
		t.Fatalf("state = %q, want %q", out, "hello")
	}

	if err := d.Signal(ctx, id, "advance", "late"); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Signal completed run = %v, want ErrRunCompleted", err)
	}
}

func TestIdempotentRetrySameKeyNoDoubleExecute(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-1"})

	var calls atomic.Int64
	d.RegisterStep("once", func(_ context.Context, input any) (any, error) {
		calls.Add(1)
		return input, nil
	})
	ctx := t.Context()

	if _, err := d.Start(ctx, "once", "first", "idem-1"); err != nil {
		t.Fatalf("first Start failed: %v", err)
	}

	if _, err := d.Start(ctx, "once", "second", "idem-1"); !errors.Is(err, workflow.ErrDuplicateRun) {
		t.Fatalf("second Start = %v, want ErrDuplicateRun", err)
	}

	if n := calls.Load(); n != 1 {
		t.Fatalf("step executed %d times, want 1", n)
	}

	var out string
	if err := d.Query(ctx, "idem-1", "state", &out); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if out != "first" {
		t.Fatalf("state = %q, want %q (original preserved)", out, "first")
	}
}

func TestLeaseExpiryReclaimBySecondOwner(t *testing.T) {
	// Two drivers share one file-backed sqlite DB (separate :memory: pools
	// cannot share state). The step blocks on a channel so the run stays
	// in "running" with driver1's lease; the test expires that lease
	// directly (no sleep), then driver2 reclaims and re-executes.
	path := filepath.Join(t.TempDir(), "wf.db")

	d1 := mustNew(t, Options{Path: path, Owner: "owner-1", LeaseTTL: time.Minute})
	d2 := mustNew(t, Options{Path: path, Owner: "owner-2", LeaseTTL: time.Minute})
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})

	d1.RegisterStep("job", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-release
		return "stale", nil
	})

	var reclaimed atomic.Int64
	d2.RegisterStep("job", func(_ context.Context, _ any) (any, error) {
		reclaimed.Add(1)
		return "fresh", nil
	})

	done := make(chan error, 1)
	go func() {
		_, err := d1.Start(ctx, "job", "input", "lease-run")
		done <- err
	}()

	<-started

	// Expire driver1's lease directly: deterministic, no sleep.
	past := time.Now().UTC().Add(-time.Hour)
	n, err := orm.UpdateTable(d1.tbl).Where(d1.cID.Eq("lease-run")).
		Set(orm.Set(d1.cExpires, past)).Exec(ctx, d1.conn)
	if err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	if n != 1 {
		t.Fatalf("expire lease affected %d rows, want 1", n)
	}

	if err := d2.Reclaim(ctx, "lease-run", "owner-2"); err != nil {
		t.Fatalf("Reclaim failed: %v", err)
	}

	if n := reclaimed.Load(); n != 1 {
		t.Fatalf("reclaim executed step %d times, want 1", n)
	}

	var out string
	if err := d2.Query(ctx, "lease-run", "state", &out); err != nil {
		t.Fatalf("Query after reclaim failed: %v", err)
	}

	if out != "fresh" {
		t.Fatalf("state = %q, want %q", out, "fresh")
	}

	close(release)

	if err := <-done; err != nil {
		t.Fatalf("stale Start returned error: %v", err)
	}

	// Stale owner must not clobber the reclaimed result.
	if err := d2.Query(ctx, "lease-run", "state", &out); err != nil {
		t.Fatalf("Query after stale finish failed: %v", err)
	}

	if out != "fresh" {
		t.Fatalf("stale step clobbered reclaim, state = %q", out)
	}
}

func TestReclaimLiveLeaseHeld(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-1", LeaseTTL: time.Hour})
	ctx := t.Context()

	started := make(chan struct{})
	release := make(chan struct{})

	d.RegisterStep("job", func(_ context.Context, _ any) (any, error) {
		close(started)
		<-release
		return "v", nil
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = d.Start(ctx, "job", "input", "held-run")
	}()

	<-started

	if err := d.Reclaim(ctx, "held-run", "owner-2"); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("Reclaim live lease = %v, want ErrLeaseHeld", err)
	}

	var held *LeaseHeldError
	if err := d.Reclaim(ctx, "held-run", "owner-2"); !errors.As(err, &held) {
		t.Fatalf("errors.As(%v, *LeaseHeldError) = false", err)
	}

	close(release)
	<-done
}

func TestStartUnknownStep(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	ctx := t.Context()

	if _, err := d.Start(ctx, "nosuchstep", "data", ""); !errors.Is(err, workflow.ErrUnknownStep) {
		t.Fatalf("Start = %v, want ErrUnknownStep", err)
	}
}

func TestQueryUnknownRun(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	ctx := t.Context()

	var out string
	if err := d.Query(ctx, "missing", "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query = %v, want ErrUnknownRun", err)
	}
}

func TestCancelCompletedRejected(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	d.RegisterStep("s", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "s", "v", "")
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := d.Cancel(ctx, id); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Cancel completed = %v, want ErrRunCompleted", err)
	}
}

func TestOptionsValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{"empty valid", Options{}, false},
		{"negative maxconns", Options{MaxConns: -1}, true},
		{"negative minconns", Options{MinConns: -1}, true},
		{"negative lifetime", Options{MaxConnLifetime: -time.Second}, true},
		{"negative idletime", Options{MaxConnIdleTime: -time.Second}, true},
		{"negative lease", Options{LeaseTTL: -time.Second}, true},
		{"bad table", Options{Table: "no-dashes!"}, true},
	}

	for _, tc := range cases {
		err := tc.opts.Validate()
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: Validate() = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

func TestRegisterReservesNameFailsClosed(t *testing.T) {
	Register()

	_, err := workflow.Open(Adapter, workflow.Options{})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Open = %v, want ErrNotConfigured", err)
	}
}

func TestImplementsContracts(t *testing.T) {
	t.Parallel()

	var _ workflow.Workflow = (*driver)(nil)
	var _ workflow.StepRegistrar = (*driver)(nil)
	var _ Reclaimer = (*driver)(nil)
}
