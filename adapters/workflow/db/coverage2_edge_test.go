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
	"github.com/zenta-dev/zever/orm/dialect"
)

// stubRowFail is an orm.Row double that fails Scan.
type stubRowFail struct{ err error }

func (s stubRowFail) Scan(_ ...any) error { return s.err }

// TestCoverOpenInvalidOptions fails closed before touching the DB.
func TestCoverOpenInvalidOptions(t *testing.T) {
	t.Parallel()

	if _, err := Open(Options{LeaseTTL: -time.Second}); err == nil {
		t.Fatal("Open(negative lease) = nil, want error")
	}
}

// TestCoverOpenBadPath fails closed when sqlite cannot open the path.
func TestCoverOpenBadPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := Open(Options{Options: coredb.Options{Path: filepath.Join(dir, "nope", "db.sqlite")}}); err == nil {
		t.Fatal("Open(bad path) = nil, want error")
	}
}

// TestCoverNewFromDBInvalidOptions fails closed on invalid options.
func TestCoverNewFromDBInvalidOptions(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: filepath.Join(t.TempDir(), "v.db")})
	if err != nil {
		t.Fatalf("sqlite New error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	if _, err := NewFromDB(conn, Options{LeaseTTL: -time.Second}); err == nil {
		t.Fatal("NewFromDB(negative lease) = nil, want error")
	}
}

// TestCoverOpenBadDSN fails closed when postgres cannot connect.
func TestCoverOpenBadDSN(t *testing.T) {
	t.Parallel()

	opts := Options{Owner: "o"}
	opts.DSN = "postgres://127.0.0.1:1/db?sslmode=disable"
	if _, err := Open(opts); err == nil {
		t.Fatal("Open(bad dsn) = nil, want error")
	}
}

// TestCoverRunRowScanFail covers Scan error paths.
func TestCoverRunRowScanFail(t *testing.T) {
	t.Parallel()

	var r runRow
	if err := r.Scan(stubRowFail{err: errors.New("scan boom")}); err == nil {
		t.Fatal("Scan(fail) = nil, want error")
	}

	// Bad timestamp text.
	badTime := stubScanRow{
		expRaw:     "not-a-time",
		createdRaw: time.Now().UTC().Format(time.RFC3339Nano),
		updatedRaw: time.Now().UTC().Format(time.RFC3339Nano),
		attemptRaw: int64(1),
	}
	if err := (&runRow{}).Scan(badTime); err == nil || !strings.Contains(err.Error(), "lease_expires_at") {
		t.Fatalf("Scan(bad exp) = %v, want lease_expires_at", err)
	}

	// Bad attempt type.
	now := time.Now().UTC().Format(time.RFC3339Nano)
	badAttempt := stubScanRow{expRaw: now, createdRaw: now, updatedRaw: now, attemptRaw: "seven"}
	if err := (&runRow{}).Scan(badAttempt); err == nil || !strings.Contains(err.Error(), "attempt") {
		t.Fatalf("Scan(bad attempt) = %v, want attempt", err)
	}

	// Bad created/updated timestamps.
	badCreated := stubScanRow{expRaw: now, createdRaw: "nope", updatedRaw: now, attemptRaw: int64(1)}
	if err := (&runRow{}).Scan(badCreated); err == nil || !strings.Contains(err.Error(), "created_at") {
		t.Fatalf("Scan(bad created) = %v, want created_at", err)
	}
	badUpdated := stubScanRow{expRaw: now, createdRaw: now, updatedRaw: "nope", attemptRaw: int64(1)}
	if err := (&runRow{}).Scan(badUpdated); err == nil || !strings.Contains(err.Error(), "updated_at") {
		t.Fatalf("Scan(bad updated) = %v, want updated_at", err)
	}
}

// stubScanRow is an orm.Row double with scripted cells.
type stubScanRow struct {
	expRaw     any
	createdRaw any
	updatedRaw any
	attemptRaw any
}

func (s stubScanRow) Scan(dest ...any) error {
	if len(dest) != 10 {
		return errors.New("db: stubScanRow wants 10 dests")
	}
	p0, ok := dest[0].(*string)
	if !ok {
		return errors.New("db: stubScanRow dest 0 wants *string")
	}
	*p0 = "wf"
	p1, ok := dest[1].(*string)
	if !ok {
		return errors.New("db: stubScanRow dest 1 wants *string")
	}
	*p1 = "step"
	p2, ok := dest[2].(*string)
	if !ok {
		return errors.New("db: stubScanRow dest 2 wants *string")
	}
	*p2 = stateRunning
	p3, ok := dest[3].(*[]byte)
	if !ok {
		return errors.New("db: stubScanRow dest 3 wants *[]byte")
	}
	*p3 = []byte(`"hi"`)
	p4, ok := dest[4].(*string)
	if !ok {
		return errors.New("db: stubScanRow dest 4 wants *string")
	}
	*p4 = "idem"
	p5, ok := dest[5].(*string)
	if !ok {
		return errors.New("db: stubScanRow dest 5 wants *string")
	}
	*p5 = "owner"
	// dest[6..9] are *any pointing at the caller's cells; assign through them.
	if p, ok := dest[6].(*any); ok {
		*p = s.expRaw
	}
	if p, ok := dest[7].(*any); ok {
		*p = s.attemptRaw
	}
	if p, ok := dest[8].(*any); ok {
		*p = s.createdRaw
	}
	if p, ok := dest[9].(*any); ok {
		*p = s.updatedRaw
	}
	return nil
}

// TestCoverCoerceTimeBadString covers unparsable timestamp text.
func TestCoverCoerceTimeBadString(t *testing.T) {
	t.Parallel()

	if _, err := coerceTime("not-a-time"); err == nil {
		t.Fatal("coerceTime(bad) = nil, want error")
	}
	if _, err := coerceTime([]byte("not-a-time")); err == nil {
		t.Fatal("coerceTime(bad bytes) = nil, want error")
	}
}

// TestCoverStartStepError removes the row and reports step failure.
func TestCoverStartStepError(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-step-err"})
	d.RegisterStep("fail", func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("step boom")
	})
	ctx := t.Context()

	if _, err := d.Start(ctx, "fail", "in", "fail-run"); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("Start(step err) = %v, want failed", err)
	}

	var out string
	if err := d.Query(ctx, "fail-run", "state", &out); !errors.Is(err, workflow.ErrUnknownRun) {
		t.Fatalf("Query(failed run) = %v, want ErrUnknownRun", err)
	}
}

// TestCoverStartDuplicateInsertRace covers the duplicate-insert path via two
// drivers sharing one file.
func TestCoverStartDuplicateInsertRace(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "dup.db")
	d1 := mustNew(t, Options{Path: path, Owner: "o1"})
	d2 := mustNew(t, Options{Path: path, Owner: "o2"})
	d1.RegisterStep("greet", echoStep)
	d2.RegisterStep("greet", echoStep)
	ctx := t.Context()

	if _, err := d1.Start(ctx, "greet", "a", "dup-run"); err != nil {
		t.Fatalf("first Start error = %v", err)
	}
	if _, err := d2.Start(ctx, "greet", "b", "dup-run"); !errors.Is(err, workflow.ErrDuplicateRun) {
		t.Fatalf("second Start = %v, want ErrDuplicateRun", err)
	}
}

// TestCoverUnsupportedDialectOps covers checkDialect failures for Signal,
// Reclaim, SagaStatus and Recover.
func TestCoverUnsupportedDialectOps(t *testing.T) {
	t.Parallel()

	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("sqlite New error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	w, err := NewFromDB(conn, Options{Owner: "owner-unsup"})
	if err != nil {
		t.Fatalf("NewFromDB error = %v", err)
	}
	d, ok := w.(*driver)
	if !ok {
		t.Fatalf("NewFromDB returned %T", w)
	}
	d.conn = &stubDB{DB: conn, dialect: "mysql"}
	ctx := t.Context()

	if err := d.Signal(ctx, "x", "q", "v"); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Signal(mysql) = %v, want ErrUnsupportedByDialect", err)
	}
	if err := d.Reclaim(ctx, "x", "o"); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Reclaim(mysql) = %v, want ErrUnsupportedByDialect", err)
	}
	if _, err := d.SagaStatus(ctx, "x"); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("SagaStatus(mysql) = %v, want ErrUnsupportedByDialect", err)
	}
	if _, err := d.RecoverStuckSagas(ctx, time.Minute); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Recover(mysql) = %v, want ErrUnsupportedByDialect", err)
	}
	if _, err := d.RunSaga(ctx, "nope", nil, ""); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("RunSaga(mysql) = %v, want ErrUnsupportedByDialect", err)
	}
}

// TestCoverRunSagaUnknown fails closed on unregistered saga names.
func TestCoverRunSagaUnknown(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga-unk"})
	if _, err := d.RunSaga(t.Context(), "missing", nil, "wf-1"); !errors.Is(err, workflow.ErrUnknownSaga) {
		t.Fatalf("RunSaga(unknown) = %v, want ErrUnknownSaga", err)
	}
}

// TestCoverRunSagaMarshalInputFail fails before inserting a row.
func TestCoverRunSagaMarshalInputFail(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga-bad"})
	d.RegisterSaga("bad-in", []workflow.SagaStep{{Name: "a"}})
	if _, err := d.RunSaga(t.Context(), "bad-in", func() {}, "wf-bad"); err == nil ||
		!strings.Contains(err.Error(), "marshal input") {
		t.Fatalf("RunSaga(func) = %v, want marshal input", err)
	}
}

// TestCoverSagaStatusUnknown reports ErrSagaNotFound.
func TestCoverSagaStatusUnknown(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-saga-st"})
	if _, err := d.SagaStatus(t.Context(), "missing"); !errors.Is(err, workflow.ErrSagaNotFound) {
		t.Fatalf("SagaStatus(missing) = %v, want ErrSagaNotFound", err)
	}
}

// TestCoverRecoverSkipsUnknownSaga skips runs whose definition vanished.
func TestCoverRecoverSkipsUnknownSaga(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-rec", LeaseTTL: time.Minute})
	ctx := t.Context()
	d.RegisterSaga("gone", []workflow.SagaStep{{Name: "a",
		Execute: func(_ context.Context, _ any) (any, error) { return "ok", nil }}})
	id, err := d.RunSaga(ctx, "gone", "in", "wf-gone")
	if err != nil {
		t.Fatalf("RunSaga error = %v", err)
	}
	// Expire lease so recovery picks it up, then drop the definition.
	past := time.Now().UTC().Add(-time.Hour)
	if _, uerr := orm.UpdateTable(d.sagaTbl).Where(d.sagaRunID.Eq(string(id))).Set(
		orm.Set(d.sagaLockedUntil, past),
	).Exec(ctx, d.conn); uerr != nil {
		t.Fatalf("expire saga lease: %v", uerr)
	}
	d.mu.Lock()
	delete(d.sagas, "gone")
	d.mu.Unlock()

	n, err := d.RecoverStuckSagas(ctx, time.Minute)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}
	if n != 0 {
		t.Fatalf("Recover = %d, want 0 (skipped unknown)", n)
	}
}

// TestCoverDecodeSagaState covers empty and corrupt documents.
func TestCoverDecodeSagaState(t *testing.T) {
	t.Parallel()

	st, err := decodeSagaState("")
	if err != nil || st == nil {
		t.Fatalf("decodeSagaState(empty) = %v,%v want empty,nil", st, err)
	}
	if _, err := decodeSagaState("{bad"); err == nil {
		t.Fatal("decodeSagaState(corrupt) = nil, want error")
	}
}

// TestCoverLoadSagaNotFound covers load helpers on missing IDs.
func TestCoverLoadSagaNotFound(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "owner-load"})
	ctx := t.Context()
	if _, err := d.loadSaga(ctx, "missing"); !errors.Is(err, workflow.ErrSagaNotFound) {
		t.Fatalf("loadSaga = %v, want ErrSagaNotFound", err)
	}
	if _, err := d.loadSagaByWorkflowID(ctx, "missing"); !errors.Is(err, workflow.ErrSagaNotFound) {
		t.Fatalf("loadSagaByWorkflowID = %v, want ErrSagaNotFound", err)
	}
	if _, err := d.loadSagaState(ctx, "missing"); !errors.Is(err, workflow.ErrSagaNotFound) {
		t.Fatalf("loadSagaState = %v, want ErrSagaNotFound", err)
	}
	if got := d.loadErr(ctx, "missing"); got != "" {
		t.Fatalf("loadErr(missing) = %q, want empty", got)
	}
	if got := d.loadSuccessfulCompensations(ctx, "missing"); len(got) != 0 {
		t.Fatalf("loadSuccessfulCompensations(missing) = %v, want empty", got)
	}
}

// TestCoverSagaScanFail covers saga row Scan error paths.
func TestCoverSagaScanFail(t *testing.T) {
	t.Parallel()

	var r sagaRunRow
	if err := r.Scan(stubRowFail{err: errors.New("saga scan boom")}); err == nil {
		t.Fatal("saga Scan(fail) = nil, want error")
	}
	var c sagaCompensationRow
	if err := c.Scan(stubRowFail{err: errors.New("comp scan boom")}); err == nil {
		t.Fatal("comp Scan(fail) = nil, want error")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, tc := range []struct {
		name string
		row  stubSagaRow
		want string
	}{
		{"locked", stubSagaRow{lockedRaw: "nope", createdRaw: now, updatedRaw: now}, "locked_until"},
		{"created", stubSagaRow{lockedRaw: now, createdRaw: "nope", updatedRaw: now}, "created_at"},
		{"updated", stubSagaRow{lockedRaw: now, createdRaw: now, updatedRaw: "nope"}, "updated_at"},
	} {
		if err := (&sagaRunRow{}).Scan(tc.row); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("saga Scan(%s) = %v, want %s", tc.name, err, tc.want)
		}
	}
}

// stubSagaRow is an orm.Row double with scripted saga cells.
type stubSagaRow struct {
	lockedRaw  any
	createdRaw any
	updatedRaw any
}

// Scan fills 11 saga destinations.
func (s stubSagaRow) Scan(dest ...any) error {
	if len(dest) != 11 {
		return errors.New("db: stubSagaRow wants 11 dests")
	}
	p0, ok := dest[0].(*string)
	if !ok {
		return errors.New("db: stubSagaRow dest 0 wants *string")
	}
	*p0 = "run"
	p1, ok := dest[1].(*string)
	if !ok {
		return errors.New("db: stubSagaRow dest 1 wants *string")
	}
	*p1 = "saga"
	p2, ok := dest[2].(*string)
	if !ok {
		return errors.New("db: stubSagaRow dest 2 wants *string")
	}
	*p2 = "wf"
	p3, ok := dest[3].(*string)
	if !ok {
		return errors.New("db: stubSagaRow dest 3 wants *string")
	}
	*p3 = "running"
	p4, ok := dest[4].(*int)
	if !ok {
		return errors.New("db: stubSagaRow dest 4 wants *int")
	}
	*p4 = 0
	p5, ok := dest[5].(*string)
	if !ok {
		return errors.New("db: stubSagaRow dest 5 wants *string")
	}
	*p5 = "{}"
	p6, ok := dest[6].(*int)
	if !ok {
		return errors.New("db: stubSagaRow dest 6 wants *int")
	}
	*p6 = -1
	p7, ok := dest[7].(*string)
	if !ok {
		return errors.New("db: stubSagaRow dest 7 wants *string")
	}
	*p7 = ""
	if p, ok := dest[8].(*any); ok {
		*p = s.lockedRaw
	}
	if p, ok := dest[9].(*any); ok {
		*p = s.createdRaw
	}
	if p, ok := dest[10].(*any); ok {
		*p = s.updatedRaw
	}
	return nil
}

// TestCoverReclaimStepError propagates step failure during reclaim.
func TestCoverReclaimStepError(t *testing.T) {
	t.Parallel()

	d := mustNew(t, Options{Owner: "o1", LeaseTTL: time.Minute})
	d.RegisterStep("flaky", func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("flaky boom")
	})
	ctx := t.Context()
	release := mustRunningRun(t, d, "flaky", "reclaim-err")
	past := time.Now().UTC().Add(-time.Hour)
	if _, err := orm.UpdateTable(d.tbl).Where(d.cID.Eq("reclaim-err")).Set(
		orm.Set(d.cExpires, past),
	).Exec(ctx, d.conn); err != nil {
		t.Fatalf("expire: %v", err)
	}
	// Swap to failing step before reclaim re-executes.
	d.RegisterStep("flaky", func(_ context.Context, _ any) (any, error) {
		return nil, errors.New("flaky boom")
	})
	if err := d.Reclaim(ctx, "reclaim-err", "o2"); err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("Reclaim(step err) = %v, want failed", err)
	}
	release()
}
