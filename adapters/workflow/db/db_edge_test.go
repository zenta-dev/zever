package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/orm"
)

// mustRunningRun starts a run whose step blocks until the returned release
// function is called, leaving the row in the running state for edge
// assertions. The release function also joins the Start goroutine.
func mustRunningRun(t *testing.T, d *driver, step, id string) func() {
	t.Helper()

	started := make(chan struct{})
	releaseCh := make(chan struct{})

	d.RegisterStep(step, func(_ context.Context, _ any) (any, error) {
		close(started)
		<-releaseCh

		return "done", nil
	})

	ctx := t.Context()
	done := make(chan error, 1)

	go func() {
		_, err := d.Start(ctx, step, "input", id)
		done <- err
	}()

	<-started

	return func() {
		close(releaseCh)

		if err := <-done; err != nil {
			t.Errorf("Start(%s) error = %v", step, err)
		}
	}
}

// TestEdgeStartNilInput stores a nil input and decodes it back as nil.
func TestEdgeStartNilInput(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "greet", nil, "nil-input")
	if err != nil {
		t.Fatalf("Start(nil) error = %v", err)
	}

	var out any
	if err := d.Query(ctx, id, "state", &out); err != nil {
		t.Fatalf("Query error = %v", err)
	}

	if out != nil {
		t.Fatalf("state = %v, want nil", out)
	}
}

// TestEdgeQueryMalformedPayload reports a JSON type mismatch when the stored
// payload cannot decode into the requested target.
func TestEdgeQueryMalformedPayload(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "greet", "hello", "mal-1")
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	var out int
	if err := d.Query(ctx, id, "state", &out); err == nil {
		t.Fatal("Query(mismatch) = nil, want error")
	} else if !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("Query(mismatch) = %q, want type mismatch", err)
	}
}

// TestEdgeQueryNonPointerTarget rejects a non-pointer decode target.
func TestEdgeQueryNonPointerTarget(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "greet", "value", "nonptr-1")
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	var out string
	if err := d.Query(ctx, id, "state", out); err == nil {
		t.Fatal("Query(non-pointer) = nil, want error")
	} else if !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("Query(non-pointer) = %q, want type mismatch", err)
	}
}

// TestEdgeQueryNilTarget rejects a nil decode target.
func TestEdgeQueryNilTarget(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "greet", "value", "niltarget-1")
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	if err := d.Query(ctx, id, "state", nil); err == nil {
		t.Fatal("Query(nil) = nil, want error")
	} else if !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("Query(nil) = %q, want type mismatch", err)
	}
}

// TestEdgeQueryUnknownName rejects an empty or unknown query name.
func TestEdgeQueryUnknownName(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "greet", "value", "qname-1")
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	var out string
	if err := d.Query(ctx, id, "", &out); !errors.Is(err, workflow.ErrUnknownQuery) {
		t.Fatalf("Query(empty) = %v, want ErrUnknownQuery", err)
	} else {
		var qErr workflow.UnknownQueryError
		if !errors.As(err, &qErr) || qErr.Query != "" {
			t.Fatalf("errors.As(%v, *UnknownQueryError) = %v, want empty Query", err, qErr)
		}
	}
}

// TestEdgeStartUnmarshalableInput fails before inserting a row and leaves the
// run ID unknown.
func TestEdgeStartUnmarshalableInput(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	if _, err := d.Start(ctx, "greet", func() {}, "bad-input"); err == nil {
		t.Fatal("Start(func input) = nil, want error")
	} else if !strings.Contains(err.Error(), "marshal input") {
		t.Fatalf("Start(func input) = %q, want marshal input", err)
	}

	var out string
	if err := d.Query(ctx, "bad-input", "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query failed run = %v, want ErrUnknownRun", err)
	}
}

// TestEdgeStartUnmarshalableResult removes the row and propagates the marshal
// error when a step returns a non-marshalable result.
func TestEdgeStartUnmarshalableResult(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{})
	d.RegisterStep("bad", func(_ context.Context, _ any) (any, error) {
		return func() {}, nil
	})
	ctx := t.Context()

	if _, err := d.Start(ctx, "bad", "ok", "bad-result"); err == nil {
		t.Fatal("Start(func result) = nil, want error")
	} else if !strings.Contains(err.Error(), "marshal result") {
		t.Fatalf("Start(func result) = %q, want marshal result", err)
	}

	var out string
	if err := d.Query(ctx, "bad-result", "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query failed run = %v, want ErrUnknownRun", err)
	}
}

// TestEdgeSignalUnmarshalableValue propagates a marshal error for a
// non-marshalable signal value on a running run.
func TestEdgeSignalUnmarshalableValue(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-sig"})
	release := mustRunningRun(t, d, "job", "sig-bad")
	ctx := t.Context()

	if err := d.Signal(ctx, "sig-bad", "advance", func() {}); err == nil {
		t.Fatal("Signal(func) = nil, want error")
	} else if !strings.Contains(err.Error(), "marshal value") {
		t.Fatalf("Signal(func) = %q, want marshal value", err)
	}

	release()
}

// TestEdgeSignalNilValue records a nil signal value on a running run.
func TestEdgeSignalNilValue(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-sig"})
	release := mustRunningRun(t, d, "job", "sig-nil")
	ctx := t.Context()

	if err := d.Signal(ctx, "sig-nil", "", nil); err != nil {
		t.Fatalf("Signal(nil) error = %v", err)
	}

	var out any
	if err := d.Query(ctx, "sig-nil", "state", &out); err != nil {
		t.Fatalf("Query mid-run error = %v", err)
	}

	if out != nil {
		t.Fatalf("mid-run state = %v, want nil", out)
	}

	release()
}

// TestEdgeReclaimEmptyOwner rejects blank owners before touching the DB.
func TestEdgeReclaimEmptyOwner(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-1"})
	ctx := t.Context()

	for _, owner := range []string{"", "   "} {
		err := d.Reclaim(ctx, "any-run", owner)
		if err == nil {
			t.Fatalf("Reclaim(owner %q) = nil, want error", owner)
		}

		if !strings.Contains(err.Error(), "non-empty owner") {
			t.Fatalf("Reclaim(owner %q) = %q, want non-empty owner", owner, err)
		}
	}
}

// TestEdgeReclaimUnknownRun reports ErrUnknownRun for a missing run.
func TestEdgeReclaimUnknownRun(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-1"})
	ctx := t.Context()

	err := d.Reclaim(ctx, "missing", "owner-2")
	if !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Reclaim(unknown) = %v, want ErrUnknownRun", err)
	}

	var unk workflow.UnknownRunError
	if !errors.As(err, &unk) {
		t.Fatalf("errors.As(%v, *UnknownRunError) = false", err)
	}
}

// TestEdgeReclaimCompletedRejected reports ErrRunCompleted for a finished run.
func TestEdgeReclaimCompletedRejected(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-1"})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	id, err := d.Start(ctx, "greet", "hi", "done-1")
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	if err := d.Reclaim(ctx, id, "owner-2"); !errors.Is(err, workflow.ErrRunCompleted) {
		t.Fatalf("Reclaim(completed) = %v, want ErrRunCompleted", err)
	}
}

// TestEdgeReclaimMalformedPayload propagates a decode error when the stored
// input is not valid JSON.
func TestEdgeReclaimMalformedPayload(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-1", LeaseTTL: time.Minute})
	release := mustRunningRun(t, d, "job", "bad-payload")
	ctx := t.Context()

	past := time.Now().UTC().Add(-time.Hour)
	n, err := orm.UpdateTable(d.tbl).Where(d.cID.Eq("bad-payload")).Set(
		orm.Set(d.cPayload, []byte("{not-json")),
		orm.Set(d.cExpires, past),
	).Exec(ctx, d.conn)
	if err != nil {
		t.Fatalf("corrupt payload: %v", err)
	}

	if n != 1 {
		t.Fatalf("corrupt payload affected %d rows, want 1", n)
	}

	if err := d.Reclaim(ctx, "bad-payload", "owner-2"); err == nil {
		t.Fatal("Reclaim(malformed) = nil, want error")
	} else if !strings.Contains(err.Error(), "decode input") {
		t.Fatalf("Reclaim(malformed) = %q, want decode input", err)
	}

	release()
}

// TestEdgeConcurrentStartDistinctIDs documents goroutine-safety: concurrent
// Starts with distinct workflow IDs all succeed and stay queryable.
func TestEdgeConcurrentStartDistinctIDs(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-conc"})
	d.RegisterStep("greet", echoStep)
	ctx := t.Context()

	const callers = 8

	var wg sync.WaitGroup

	errs := make([]error, callers)

	for i := 0; i < callers; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			_, errs[i] = d.Start(ctx, "greet", i, fmt.Sprintf("conc-%d", i))
		}(i)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("Start(conc-%d) error = %v", i, err)
		}

		var out int
		if err := d.Query(ctx, workflow.RunID(fmt.Sprintf("conc-%d", i)), "state", &out); err != nil {
			t.Fatalf("Query(conc-%d) error = %v", i, err)
		}

		if out != i {
			t.Fatalf("Query(conc-%d) = %d, want %d", i, out, i)
		}
	}
}
