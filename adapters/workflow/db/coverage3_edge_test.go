package db

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/orm"
)

// stubPingFail is a coredb.DB double failing Ping.
type stubPingFail struct {
	coredb.DB
}

// Ping fails closed without touching the network.
func (s stubPingFail) Ping(_ context.Context) error { return errors.New("db: ping boom") }

// TestCoverOpenFromDBPingFail covers openFromDB Ping failure.
func TestCoverOpenFromDBPingFail(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "p.db")})
	if err != nil {
		t.Fatalf("sqlite New error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	if _, err := NewFromDB(stubPingFail{DB: conn}, Options{Owner: "o"}); err == nil ||
		!strings.Contains(err.Error(), "ping boom") {
		t.Fatalf("NewFromDB(ping fail) = %v, want ping boom", err)
	}
}

// TestCoverReclaimUnknownStep fails closed when the step vanished.
func TestCoverReclaimUnknownStep(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "o1", LeaseTTL: time.Minute})
	release := mustRunningRun(t, d, "job", "unk-step")
	ctx := t.Context()

	past := time.Now().UTC().Add(-time.Hour)
	if _, err := orm.UpdateTable(d.tbl).Where(d.cID.Eq("unk-step")).Set(
		orm.Set(d.cExpires, past),
	).Exec(ctx, d.conn); err != nil {
		t.Fatalf("expire: %v", err)
	}

	d.mu.Lock()
	delete(d.steps, "job")
	d.mu.Unlock()

	if err := d.Reclaim(ctx, "unk-step", "o2"); !errors.Is(err, workflow.ErrUnknownStep) {
		t.Fatalf("Reclaim(unknown step) = %v, want ErrUnknownStep", err)
	}

	// Restore for clean release.
	d.RegisterStep("job", func(_ context.Context, _ any) (any, error) { return "done", nil })
	release()
}

// TestCoverReclaimMarshalResultFail propagates marshal errors.
func TestCoverReclaimMarshalResultFail(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "o1", LeaseTTL: time.Minute})
	release := mustRunningRun(t, d, "job", "bad-res")
	ctx := t.Context()

	past := time.Now().UTC().Add(-time.Hour)
	if _, err := orm.UpdateTable(d.tbl).Where(d.cID.Eq("bad-res")).Set(
		orm.Set(d.cExpires, past),
	).Exec(ctx, d.conn); err != nil {
		t.Fatalf("expire: %v", err)
	}

	d.RegisterStep("job", func(_ context.Context, _ any) (any, error) { return func() {}, nil })

	if err := d.Reclaim(ctx, "bad-res", "o2"); err == nil || !strings.Contains(err.Error(), "marshal result") {
		t.Fatalf("Reclaim(bad result) = %v, want marshal result", err)
	}

	d.RegisterStep("job", func(_ context.Context, _ any) (any, error) { return "done", nil })
	release()
}

// TestCoverRunSagaResumeCorruptState fails closed on corrupt persisted state.
func TestCoverRunSagaResumeCorruptState(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "o"})
	ctx := t.Context()
	d.RegisterSaga("ok", []workflow.SagaStep{{Name: "a",
		Execute: func(_ context.Context, _ any) (any, error) { return "ok", nil }}})
	id, err := d.RunSaga(ctx, "ok", "in", "wf-corrupt")
	if err != nil {
		t.Fatalf("RunSaga error = %v", err)
	}
	_ = id

	if _, err := d.conn.Exec(ctx, `UPDATE "workflow_saga_runs" SET status = 'running', state = '{bad' WHERE workflow_id = 'wf-corrupt'`); err != nil {
		t.Fatalf("corrupt saga state: %v", err)
	}

	if _, err := d.RunSaga(ctx, "ok", "in", "wf-corrupt"); err == nil ||
		!strings.Contains(err.Error(), "decode saga state") {
		t.Fatalf("RunSaga(corrupt resume) = %v, want decode saga state", err)
	}
}

// TestCoverRecoverCorruptState fails closed on corrupt stuck runs.
func TestCoverRecoverCorruptState(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "o", LeaseTTL: time.Minute})
	ctx := t.Context()
	d.RegisterSaga("ok", []workflow.SagaStep{{Name: "a",
		Execute: func(_ context.Context, _ any) (any, error) { return "ok", nil }}})
	if _, err := d.RunSaga(ctx, "ok", "in", "wf-rec-corrupt"); err != nil {
		t.Fatalf("RunSaga error = %v", err)
	}

	past := time.Now().UTC().Add(-time.Hour)
	if _, err := d.conn.Exec(ctx, `UPDATE "workflow_saga_runs" SET status = 'running', state = '{bad', locked_until = '`+past.Format(time.RFC3339Nano)+`' WHERE workflow_id = 'wf-rec-corrupt'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	if _, err := d.RecoverStuckSagas(ctx, time.Minute); err == nil ||
		!strings.Contains(err.Error(), "decode saga state") {
		t.Fatalf("Recover(corrupt) = %v, want decode saga state", err)
	}
}

// TestCoverLoadHelpersDBError covers load error paths via closed conn.
func TestCoverLoadHelpersDBError(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "o"})
	ctx := t.Context()
	d.RegisterStep("greet", echoStep)
	if _, err := d.Start(ctx, "greet", "hi", "load-err"); err != nil {
		t.Fatalf("Start error = %v", err)
	}
	// Close underlying pool; loads must fail closed (not panic).
	if err := d.conn.Close(ctx); err != nil {
		t.Fatalf("conn Close error = %v", err)
	}

	if _, err := d.load(ctx, "load-err"); err == nil {
		t.Fatal("load(closed) = nil, want error")
	}
	if _, err := d.loadSaga(ctx, "missing"); err == nil {
		t.Fatal("loadSaga(closed) = nil, want error")
	}
	if got := d.loadSuccessfulCompensations(ctx, "missing"); got != nil {
		t.Fatalf("loadSuccessfulCompensations(closed) = %v, want nil", got)
	}
}
