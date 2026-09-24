package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

// newUpsertDB mirrors newWidgetsDB (query_helpers_test.go) but creates the
// widgets table with a UNIQUE constraint on id -- ON CONFLICT needs a
// unique/primary-key target to collide on, which the shared widgets fixture
// deliberately lacks.
func newUpsertDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id text UNIQUE, name text, quantity integer, bio text)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	if _, err := conn.Exec(ctx, `INSERT INTO widgets (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, "w1", "Alpha", int64(10), "first"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	return ctx, conn
}

// recordingDB wraps a db.DB and records the last statement it executed, so
// round-trip tests can assert both the rendered SQL and the scanned values.
type recordingDB struct {
	db.DB

	query string
}

func (r *recordingDB) Query(ctx context.Context, query string, args ...any) (db.Rows, error) {
	r.query = query

	return r.DB.Query(ctx, query, args...) //nolint:wrapcheck // test double, error passes straight through
}

func (r *recordingDB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	r.query = query

	return r.DB.Exec(ctx, query, args...) //nolint:wrapcheck // test double, error passes straight through
}

// upsertWidget is the 4-column upsert used throughout the round-trip tests.
func upsertWidget(id, name string, qty int64, bio *string) Insert[widget] {
	assignments := []Assignment[widget]{
		Set(widgetID, id),
		Set(widgetName, name),
		Set(widgetQty, qty),
	}

	if bio == nil {
		assignments = append(assignments, widgetBio.SetNull())
	} else {
		assignments = append(assignments, widgetBio.SetValue(*bio))
	}

	return InsertInto(widgets).Values(assignments...)
}

// TestUpsertInsertOnConflict proves the conflict-absent path inserts the new
// row rather than updating anything.
func TestUpsertInsertOnConflict(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	bio := "newcomer"
	up := upsertWidget("w9", "Niner", int64(90), &bio).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetName, "UPDATED"), Set(widgetQty, int64(1)))

	if err := up.Exec(ctx, conn); err != nil {
		t.Fatalf("upsert.Exec: %v", err)
	}

	row, ok, err := From(widgets).Where(widgetID.Eq("w9")).First(ctx, conn)
	if err != nil {
		t.Fatalf("First: %v", err)
	}

	if !ok || row.Name != "Niner" || row.Quantity != 90 {
		t.Fatalf("row after absent-conflict upsert = %+v, ok=%v, want Niner/90 (inserted, not updated)", row, ok)
	}
}

// TestUpsertUpdateOnConflict proves the conflict-present path applies the
// DO UPDATE SET assignments to the existing row while leaving unassigned
// columns (bio) untouched.
func TestUpsertUpdateOnConflict(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	bio := "should-not-appear"
	up := upsertWidget("w1", "Renamed", int64(99), &bio).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetName, "Renamed"), Set(widgetQty, int64(99)))

	if err := up.Exec(ctx, conn); err != nil {
		t.Fatalf("upsert.Exec: %v", err)
	}

	row, ok, err := From(widgets).Where(widgetID.Eq("w1")).First(ctx, conn)
	if err != nil {
		t.Fatalf("First: %v", err)
	}

	if !ok || row.Name != "Renamed" || row.Quantity != 99 {
		t.Fatalf("row after conflict upsert = %+v, ok=%v, want Renamed/99", row, ok)
	}

	bioVal, bioOK := row.Bio.Get()
	if !bioOK || bioVal != "first" {
		t.Fatalf("row.Bio = (%q, %v), want (\"first\", true) -- bio was not in the SET list and must be untouched", bioVal, bioOK)
	}
}

// TestUpsertDoNothing proves DO NOTHING leaves a conflicting row untouched
// but still inserts a non-conflicting one.
func TestUpsertDoNothing(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	bio := "ignored"
	if err := upsertWidget("w1", "ShouldNotChange", int64(1), &bio).
		OnConflict(widgetID.Col()).
		DoNothing().
		Exec(ctx, conn); err != nil {
		t.Fatalf("upsert.Exec: %v", err)
	}

	row, ok, err := From(widgets).Where(widgetID.Eq("w1")).First(ctx, conn)
	if err != nil {
		t.Fatalf("First: %v", err)
	}

	if !ok || row.Name != "Alpha" || row.Quantity != 10 {
		t.Fatalf("row after DO NOTHING conflict = %+v, ok=%v, want untouched Alpha/10", row, ok)
	}

	upsertErr := upsertWidget("w9", "Niner", int64(90), &bio).
		OnConflict(widgetID.Col()).
		DoNothing().
		Exec(ctx, conn)
	if upsertErr != nil {
		t.Fatalf("upsert.Exec: %v", upsertErr)
	}

	exists, err := From(widgets).Where(widgetID.Eq("w9")).Exists(ctx, conn)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if !exists {
		t.Fatalf("non-conflicting DO NOTHING upsert did not insert w9")
	}
}

// TestUpsertReturningInsert asserts both the rendered SQL and the scanned
// row for the conflict-absent RETURNING path.
func TestUpsertReturningInsert(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	rec := &recordingDB{DB: conn}

	bio := "newcomer"
	got, err := upsertWidget("w9", "Niner", int64(90), &bio).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetName, "UPDATED"), Set(widgetQty, int64(1))).
		Returning().
		ExecReturning(ctx, rec)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	wantSQL := `INSERT INTO "widgets" ("id", "name", "quantity", "bio") VALUES (?, ?, ?, ?) ON CONFLICT ("id") DO UPDATE SET "name" = ?, "quantity" = ? RETURNING "id", "name", "quantity", "bio"`
	if rec.query != wantSQL {
		t.Fatalf("query = %q, want %q", rec.query, wantSQL)
	}

	if len(got) != 1 {
		t.Fatalf("ExecReturning returned %d rows, want 1", len(got))
	}

	if got[0].ID != "w9" || got[0].Name != "Niner" || got[0].Quantity != 90 {
		t.Fatalf("returned row = %+v, want inserted w9/Niner/90", got[0])
	}

	bioVal, bioOK := got[0].Bio.Get()
	if !bioOK || bioVal != "newcomer" {
		t.Fatalf("returned row.Bio = (%q, %v), want (\"newcomer\", true)", bioVal, bioOK)
	}
}

// TestUpsertReturningUpdate proves the conflict-present RETURNING path
// returns the updated row, not the pre-update one.
func TestUpsertReturningUpdate(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	bio := "discarded"
	got, err := upsertWidget("w1", "Renamed", int64(99), &bio).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetName, "Renamed"), Set(widgetQty, int64(99))).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("ExecReturning returned %d rows, want 1", len(got))
	}

	if got[0].ID != "w1" || got[0].Name != "Renamed" || got[0].Quantity != 99 {
		t.Fatalf("returned row = %+v, want updated w1/Renamed/99", got[0])
	}

	// The DO UPDATE SET list never touched bio, so the returned row carries
	// the pre-existing value.
	bioVal, bioOK := got[0].Bio.Get()
	if !bioOK || bioVal != "first" {
		t.Fatalf("returned row.Bio = (%q, %v), want (\"first\", true)", bioVal, bioOK)
	}
}

// TestUpsertReturningDoNothing proves a DO NOTHING conflict yields no
// RETURNING row at all -- the conflicting row was left alone, so there is
// nothing to return.
func TestUpsertReturningDoNothing(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	bio := "ignored"
	got, err := upsertWidget("w1", "ShouldNotChange", int64(1), &bio).
		OnConflict(widgetID.Col()).
		DoNothing().
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("ExecReturning returned %d rows, want 0 for a DO NOTHING conflict", len(got))
	}
}

// TestUpsertMultiRowReturning proves a multi-row upsert with RETURNING
// returns one row per upserted row -- inserted or updated.
func TestUpsertMultiRowReturning(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	insert := InsertInto(widgets).
		Values(Set(widgetID, "m1"), Set(widgetName, "M1"), Set(widgetQty, int64(1)), widgetBio.SetNull()).
		Values(Set(widgetID, "m2"), Set(widgetName, "M2"), Set(widgetQty, int64(2)), widgetBio.SetNull()).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetName, "UpdatedM"), Set(widgetQty, int64(0))).
		Returning()

	got, err := insert.ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("ExecReturning (insert both): %v", err)
	}

	if len(got) != 2 || got[0].ID != "m1" || got[1].ID != "m2" {
		t.Fatalf("first ExecReturning = %+v, want [m1 m2]", got)
	}

	// Re-upsert the same ids: now both conflict and update, and RETURNING
	// reports the updated rows.
	got2, err := insert.ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("ExecReturning (update both): %v", err)
	}

	if len(got2) != 2 || got2[0].Name != "UpdatedM" || got2[1].Name != "UpdatedM" {
		t.Fatalf("second ExecReturning = %+v, want both updated to UpdatedM", got2)
	}
}

// TestUpsertOnConflictDoesNotShareBackingArray proves OnConflict/DoUpdate
// keep the copy-on-write discipline: two upserts branched from one
// OnConflict never share their target or sets backing arrays.
func TestUpsertOnConflictDoesNotShareBackingArray(t *testing.T) {
	base := upsertWidget("w1", "Alpha", int64(10), nil).
		OnConflict(widgetID.Col())

	branchA := base.DoUpdate(Set(widgetName, "A"))
	branchB := base.DoUpdate(Set(widgetName, "B"))

	if branchA.conflict.target[0] != "id" || branchB.conflict.target[0] != "id" {
		t.Fatalf("targets = (%v, %v), want both [id]", branchA.conflict.target, branchB.conflict.target)
	}

	if branchA.conflict.sets[0].Value != "A" || branchB.conflict.sets[0].Value != "B" {
		t.Fatalf("branch sets mutated each other: (%v, %v)", branchA.conflict.sets, branchB.conflict.sets)
	}
}

// TestUpsertReturningDoesNotShareBackingArray proves Returning's column
// list is a fresh slice, so two branches from one base cannot corrupt each
// other.
func TestUpsertReturningDoesNotShareBackingArray(t *testing.T) {
	base := upsertWidget("w1", "Alpha", int64(10), nil).OnConflict(widgetID.Col()).DoNothing()

	branchA := base.Returning(widgetID.Col())
	branchB := base.Returning(widgetID.Col(), widgetName.Col())

	if len(base.returning) != 0 {
		t.Fatalf("base.returning mutated by branching: %v", base.returning)
	}

	if len(branchA.returning) != 1 || len(branchB.returning) != 2 {
		t.Fatalf("branch returning lengths = (%d, %d), want (1, 2)", len(branchA.returning), len(branchB.returning))
	}
}

// TestUpsertCapabilityGate drives the RETURNING path through a dialect that
// cannot support it -- a base-only dialect (no ReturningDialect method set)
// -- asserting the typed dialect.ErrUnsupportedByDialect, never a panic or a
// silent wrong-SQL fallback.
func TestUpsertReturningCapabilityGate(t *testing.T) {
	ctx := t.Context()

	up := upsertWidget("w1", "Alpha", int64(10), nil).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetQty, int64(99))).
		Returning()

	_, err := up.ExecReturning(ctx, mockExec{dialectName: "mock-nocap"})
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestUpsertExecRejectsReturning proves Exec refuses to silently drop a
// requested RETURNING clause -- the caller must use ExecReturning.
func TestUpsertExecRejectsReturning(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	err := upsertWidget("w9", "Niner", int64(90), nil).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetQty, int64(1))).
		Returning().
		Exec(ctx, conn)
	if err == nil {
		t.Fatalf("Exec after Returning() succeeded, want an error directing to ExecReturning")
	}
}

// TestUpsertExecReturningRequiresReturning proves ExecReturning refuses to
// run when no RETURNING columns were requested.
func TestUpsertExecReturningRequiresReturning(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	_, err := upsertWidget("w9", "Niner", int64(90), nil).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetQty, int64(1))).
		ExecReturning(ctx, conn)
	if err == nil {
		t.Fatalf("ExecReturning without Returning() succeeded, want an error")
	}
}

// TestUpsertDoUpdateRequiresSets proves DO UPDATE with an empty assignment
// list is rejected at exec time -- `DO UPDATE SET` with nothing to set is
// invalid SQL.
func TestUpsertDoUpdateRequiresSets(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	err := upsertWidget("w1", "Alpha", int64(10), nil).
		OnConflict(widgetID.Col()).
		DoUpdate().
		Exec(ctx, conn)
	if err == nil {
		t.Fatalf("DoUpdate() with no sets succeeded, want an error")
	}
}

// TestInsertReturningPlainPath proves a NON-conflict INSERT's Returning
// clause is no longer silently dropped: the plain path renders RETURNING
// (mirroring the ON CONFLICT path) and ExecReturning hands back the
// inserted row.
func TestInsertReturningPlainPath(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	rec := &recordingDB{DB: conn}

	got, err := InsertInto(widgets).
		Values(Set(widgetID, "w9"), Set(widgetName, "Niner"), Set(widgetQty, int64(90)), widgetBio.SetValue("newcomer")).
		Returning().
		ExecReturning(ctx, rec)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	wantSQL := `INSERT INTO "widgets" ("id", "name", "quantity", "bio") VALUES (?, ?, ?, ?) RETURNING "id", "name", "quantity", "bio"`
	if rec.query != wantSQL {
		t.Fatalf("query = %q, want %q", rec.query, wantSQL)
	}

	if len(got) != 1 {
		t.Fatalf("ExecReturning returned %d rows, want 1", len(got))
	}

	if got[0].ID != "w9" || got[0].Name != "Niner" || got[0].Quantity != 90 {
		t.Fatalf("returned row = %+v, want inserted w9/Niner/90", got[0])
	}

	bio, bioOK := got[0].Bio.Get()
	if !bioOK || bio != "newcomer" {
		t.Fatalf("returned row.Bio = (%q, %v), want (\"newcomer\", true)", bio, bioOK)
	}
}

// TestInsertReturningPlainManyRow proves the plain multi-row path renders
// RETURNING too, returning one row per inserted row.
func TestInsertReturningPlainManyRow(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	got, err := InsertInto(widgets).
		Values(Set(widgetID, "m1"), Set(widgetName, "M1"), Set(widgetQty, int64(1)), widgetBio.SetNull()).
		Values(Set(widgetID, "m2"), Set(widgetName, "M2"), Set(widgetQty, int64(2)), widgetBio.SetNull()).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	if len(got) != 2 || got[0].ID != "m1" || got[1].ID != "m2" {
		t.Fatalf("ExecReturning = %+v, want [m1 m2]", got)
	}
}

// TestInsertReturningPlainCapabilityGate proves a plain INSERT's ExecReturning
// still fails with the typed dialect.ErrUnsupportedByDialect on a dialect
// without RETURNING -- the new plain-path rendering never bypasses the
// RETURNING capability gate.
func TestInsertReturningPlainCapabilityGate(t *testing.T) {
	ctx := t.Context()

	_, err := InsertInto(widgets).
		Values(Set(widgetID, "w9"), Set(widgetName, "Niner"), Set(widgetQty, int64(90)), widgetBio.SetNull()).
		Returning().
		ExecReturning(ctx, mockExec{dialectName: "mock-nocap"})
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestUpsertRenderErrorPaths proves column-list, conflict-render, resolve
// and execution-tail failures surface instead of invalid SQL.
func TestUpsertRenderErrorPaths(t *testing.T) {
	ctx, conn := newUpsertDB(t)

	if err := InsertInto(widgets).Values().Values(Set(widgetID, "x")).Exec(ctx, conn); err == nil {
		t.Fatal("Exec with an empty first Values row succeeded, want a no-columns error")
	}

	multi := From(widgetOrders)

	targeted := upsertWidget("w1", "Alpha", int64(10), nil).
		OnConflict(widgetID.Col()).
		Where(widgetID.InSub(multi)).
		DoNothing().
		Returning()

	if _, err := targeted.ExecReturning(ctx, conn); err == nil {
		t.Fatal("ExecReturning with a multi-column conflict target predicate succeeded, want a render error")
	}

	many := InsertInto(widgets).
		Values(Set(widgetID, "m1"), Set(widgetName, "M1"), Set(widgetQty, int64(1)), widgetBio.SetNull()).
		Values(Set(widgetID, "m2"), Set(widgetName, "M2"), Set(widgetQty, int64(2)), widgetBio.SetNull()).
		OnConflict(widgetID.Col()).
		Where(widgetID.InSub(multi)).
		DoNothing().
		Returning()

	if _, err := many.ExecReturning(ctx, conn); err == nil {
		t.Fatal("multi-row ExecReturning with a multi-column conflict target predicate succeeded, want a render error")
	}

	if _, err := upsertWidget("w1", "Alpha", int64(10), nil).OnConflict(widgetID.Col()).DoUpdate(Set(widgetQty, int64(1))).Returning().ExecReturning(ctx, fakeDB{}); err == nil {
		t.Fatal("ExecReturning on an unresolvable dialect succeeded, want an error")
	}
}

// TestExecReturningTailErrorPaths drives query, scan, iteration and close
// failures through the shared RETURNING execution tail.
func TestExecReturningTailErrorPaths(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("boom")

	base := upsertWidget("w1", "Alpha", int64(10), nil).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetQty, int64(1))).
		Returning()

	queryErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, queryErr: boom}

	if _, err := base.ExecReturning(ctx, queryErr); !errors.Is(err, boom) {
		t.Fatalf("ExecReturning err = %v, want errors.Is(err, boom)", err)
	}

	scanErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{values: [][]any{{"w1", "Alpha", int64(10), "first"}}, scanErr: boom}}

	if _, err := base.ExecReturning(ctx, scanErr); !errors.Is(err, boom) {
		t.Fatalf("ExecReturning err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{
		values:  [][]any{{"w1", "Alpha", int64(10), "first"}},
		iterErr: boom,
	}}

	if _, err := base.ExecReturning(ctx, iterErr); !errors.Is(err, boom) {
		t.Fatalf("ExecReturning err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{closeErr: boom}}

	if _, err := base.ExecReturning(ctx, closeErr); !errors.Is(err, boom) {
		t.Fatalf("ExecReturning err = %v, want errors.Is(err, boom)", err)
	}
}
