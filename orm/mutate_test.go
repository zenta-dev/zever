package orm

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

// TestInsertValuesDoesNotShareBackingArray is the branching-safety proof
// for Insert[T], mirroring TestQueryOrderByDoesNotShareBackingArray in
// query_test.go: two Insert chains branched from one base via Values must
// never share (or corrupt) each other's rows.
func TestInsertValuesDoesNotShareBackingArray(t *testing.T) {
	base := InsertInto(widgets).Values(Set(widgetID, "w1"), Set(widgetName, "Alpha"))

	branchA := base.Values(Set(widgetID, "w2"), Set(widgetName, "Beta"))
	branchB := base.Values(Set(widgetID, "w3"), Set(widgetName, "Gamma"))

	if base.rowsLen != 1 {
		t.Fatalf("base.rows mutated by branching: %d rows, want 1", base.rowsLen)
	}

	rowsA := branchA.rowsSlice()
	if len(rowsA) != 2 || rowsA[1][0] != "w2" {
		t.Fatalf("branchA.rows = %v, want second row id=w2", rowsA)
	}

	rowsB := branchB.rowsSlice()
	if len(rowsB) != 2 || rowsB[1][0] != "w3" {
		t.Fatalf("branchB.rows = %v, want second row id=w3", rowsB)
	}

	if rowsA[1][0] == rowsB[1][0] {
		t.Fatalf("branchA and branchB unexpectedly share a row")
	}
}

// TestUpdateSetDoesNotShareBackingArray is Update[T]'s equivalent
// branch-safety proof.
func TestUpdateSetDoesNotShareBackingArray(t *testing.T) {
	base := UpdateTable(widgets).Set(Set(widgetName, "Base"))

	branchA := base.Set(Set(widgetQty, int64(1)))
	branchB := base.Set(Set(widgetQty, int64(2)))

	if len(base.sets) != 1 {
		t.Fatalf("base.sets mutated by branching: %v", base.sets)
	}

	if len(branchA.sets) != 2 || branchA.sets[1].Value != int64(1) {
		t.Fatalf("branchA.sets = %v, want second assignment quantity=1", branchA.sets)
	}

	if len(branchB.sets) != 2 || branchB.sets[1].Value != int64(2) {
		t.Fatalf("branchB.sets = %v, want second assignment quantity=2", branchB.sets)
	}
}

// TestUpdateWhereDoesNotMutateBase proves Update[T].Where's copy-on-write
// rule, mirroring TestQueryWhereDoesNotMutateBase.
func TestUpdateWhereDoesNotMutateBase(t *testing.T) {
	base := UpdateTable(widgets)

	branch := base.Where(widgetID.Eq("w1"))

	if base.where.IsSet() {
		t.Fatalf("base.where.IsSet() = true after branching, want false (unmodified)")
	}

	if !branch.where.IsSet() {
		t.Fatalf("branch.where.IsSet() = false, want true")
	}
}

// TestDeleteWhereDoesNotMutateBase proves Delete[T].Where's copy-on-write
// rule.
func TestDeleteWhereDoesNotMutateBase(t *testing.T) {
	base := DeleteFrom(widgets)

	branch := base.Where(widgetID.Eq("w1"))

	if base.where.IsSet() {
		t.Fatalf("base.where.IsSet() = true after branching, want false (unmodified)")
	}

	if !branch.where.IsSet() {
		t.Fatalf("branch.where.IsSet() = false, want true")
	}
}

// TestMutationRoundTrip proves Insert[T]/Update[T]/Delete[T]/Query[T]
// compose end to end against real SQLite: insert a row, update it, verify
// via Select, delete it, verify it's gone.
func TestMutationRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	if err := InsertInto(widgets).Values(
		Set(widgetID, "w4"),
		Set(widgetName, "Delta"),
		Set(widgetQty, int64(40)),
		widgetBio.SetValue("fourth"),
	).Exec(ctx, conn); err != nil {
		t.Fatalf("Insert.Exec: %v", err)
	}

	row, ok, err := From(widgets).Where(widgetID.Eq("w4")).First(ctx, conn)
	if err != nil {
		t.Fatalf("First after insert: %v", err)
	}

	if !ok || row.Name != "Delta" || row.Quantity != 40 {
		t.Fatalf("row after insert = %+v, ok=%v, want Delta/40", row, ok)
	}

	bio, bioOK := row.Bio.Get()
	if !bioOK || bio != "fourth" {
		t.Fatalf("row.Bio after insert = (%q, %v), want (\"fourth\", true)", bio, bioOK)
	}

	n, err := UpdateTable(widgets).
		Where(widgetID.Eq("w4")).
		Set(Set(widgetName, "Delta2"), widgetBio.SetNull()).
		Exec(ctx, conn)
	if err != nil {
		t.Fatalf("Update.Exec: %v", err)
	}

	if n != 1 {
		t.Fatalf("Update.Exec rows affected = %d, want 1", n)
	}

	row2, ok2, err := From(widgets).Where(widgetID.Eq("w4")).First(ctx, conn)
	if err != nil {
		t.Fatalf("First after update: %v", err)
	}

	if !ok2 || row2.Name != "Delta2" {
		t.Fatalf("row after update = %+v, ok=%v, want Delta2", row2, ok2)
	}

	if row2.Bio.IsSome() {
		t.Fatalf("row2.Bio.IsSome() = true after SetNull, want false")
	}

	n2, err := DeleteFrom(widgets).Where(widgetID.Eq("w4")).Exec(ctx, conn)
	if err != nil {
		t.Fatalf("Delete.Exec: %v", err)
	}

	if n2 != 1 {
		t.Fatalf("Delete.Exec rows affected = %d, want 1", n2)
	}

	exists, err := From(widgets).Where(widgetID.Eq("w4")).Exists(ctx, conn)
	if err != nil {
		t.Fatalf("Exists after delete: %v", err)
	}

	if exists {
		t.Fatalf("Exists after delete = true, want false")
	}
}

// TestInsertMultiRowExec proves Insert[T].Exec renders and runs a
// multi-row INSERT when Values is called more than once.
func TestInsertMultiRowExec(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	err := InsertInto(widgets).
		Values(Set(widgetID, "m1"), Set(widgetName, "M1"), Set(widgetQty, int64(1)), widgetBio.SetNull()).
		Values(Set(widgetID, "m2"), Set(widgetName, "M2"), Set(widgetQty, int64(2)), widgetBio.SetNull()).
		Exec(ctx, conn)
	if err != nil {
		t.Fatalf("Insert.Exec: %v", err)
	}

	n, err := From(widgets).Where(widgetID.In("m1", "m2")).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if n != 2 {
		t.Fatalf("Count(m1,m2) = %d, want 2", n)
	}
}

// TestDeleteWithoutWhereDeletesEverything documents Delete[T]'s
// no-guard-in-this-phase behavior: an Exec with no Where deletes every
// row, matching render.Delete's contract.
func TestDeleteWithoutWhereDeletesEverything(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	n, err := DeleteFrom(widgets).Exec(ctx, conn)
	if err != nil {
		t.Fatalf("Delete.Exec: %v", err)
	}

	if n != 3 {
		t.Fatalf("Delete.Exec rows affected = %d, want 3", n)
	}

	remaining, err := From(widgets).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}

	if remaining != 0 {
		t.Fatalf("Count after unfiltered delete = %d, want 0", remaining)
	}
}

// TestMutationUnsupportedDialect proves Insert/Update/Delete's Exec all
// surface the same dialect-resolution error Query[T].All does, rather
// than panicking or silently falling back, mirroring
// TestQueryUnsupportedDialect.
func TestMutationUnsupportedDialect(t *testing.T) {
	var fake fakeDB

	if err := InsertInto(widgets).Values(Set(widgetID, "x")).Exec(t.Context(), fake); err == nil {
		t.Fatalf("Insert.Exec with an unsupported dialect succeeded, want an error")
	}

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Exec(t.Context(), fake); err == nil {
		t.Fatalf("Update.Exec with an unsupported dialect succeeded, want an error")
	}

	if _, err := DeleteFrom(widgets).Exec(t.Context(), fake); err == nil {
		t.Fatalf("Delete.Exec with an unsupported dialect succeeded, want an error")
	}
}

var _ db.DB = fakeDB{}

// TestUpdateJoinDoesNotShareBackingArray is Update[T].Join's branch-safety
// proof: two Update chains branched from one base via Join never share
// (or corrupt) each other's join slices.
func TestUpdateJoinDoesNotShareBackingArray(t *testing.T) {
	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	base := UpdateTable(widgets)

	branchA := base.Join(rel, InnerJoin).Set(Set(widgetName, "A"))
	branchB := base.Join(rel, InnerJoin).Set(Set(widgetName, "B"))

	if len(base.joins) != 0 {
		t.Fatalf("base.joins mutated by branching: %v", base.joins)
	}

	if len(branchA.joins) != 1 || len(branchB.joins) != 1 {
		t.Fatalf("branch join lengths = (%d, %d), want (1, 1)", len(branchA.joins), len(branchB.joins))
	}

	if branchA.sets[0].Value != "A" || branchB.sets[0].Value != "B" {
		t.Fatalf("branch sets corrupted each other: (%v, %v)", branchA.sets, branchB.sets)
	}
}

// TestUpdateJoinRoundTrip proves a joined UPDATE against real SQLite: w1
// (two orders) and w2 (one order) match the join and get updated, w3 (no
// orders) does not. SQLite reports rows-affected per distinct target row
// updated (2), not per joined match.
func TestUpdateJoinRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	seedWidgetOrders(ctx, t, conn)

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	n, err := UpdateTable(widgets).
		Join(rel, InnerJoin).
		Set(Set(widgetQty, int64(999))).
		Exec(ctx, conn)
	if err != nil {
		t.Fatalf("Update.Exec: %v", err)
	}

	if n != 2 {
		t.Fatalf("Update.Exec rows affected = %d, want 2 (w1 and w2)", n)
	}

	rows, err := From(widgets).OrderBy(widgetID.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if rows[0].Quantity != 999 || rows[1].Quantity != 999 {
		t.Fatalf("w1/w2 quantities = (%d, %d), want (999, 999)", rows[0].Quantity, rows[1].Quantity)
	}

	if rows[2].Quantity != 30 {
		t.Fatalf("w3 quantity = %d, want 30 (no matching order, untouched)", rows[2].Quantity)
	}
}

// TestUpdateJoinRenderedSQL asserts the exact SQLite SQL a joined UPDATE
// renders, via recordingDB.
func TestUpdateJoinRenderedSQL(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	seedWidgetOrders(ctx, t, conn)

	rec := &recordingDB{DB: conn}

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	if _, err := UpdateTable(widgets).
		Join(rel, InnerJoin).
		Where(widgetName.Eq("Alpha")).
		Set(Set(widgetQty, int64(1))).
		Exec(ctx, rec); err != nil {
		t.Fatalf("Update.Exec: %v", err)
	}

	want := `UPDATE "widgets" SET "quantity" = ? FROM "widget_orders" WHERE "widgets"."id" = "widget_orders"."widget_id" AND "widgets"."name" = ?`
	if rec.query != want {
		t.Fatalf("query = %q, want %q", rec.query, want)
	}
}

// TestDeleteJoinUnsupportedOnSQLite proves SQLite rejects a joined DELETE
// with the typed dialect.ErrUnsupportedByDialect -- SQLite has no
// DELETE...USING, so it must never be silently rendered.
func TestDeleteJoinUnsupportedOnSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	seedWidgetOrders(ctx, t, conn)

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	_, err := DeleteFrom(widgets).Join(rel, InnerJoin).Exec(ctx, conn)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestUpdateJoinLeftUnsupportedOnSQLite proves a LEFT JOIN in a joined
// UPDATE is rejected on SQLite with the typed error -- the UPDATE...FROM
// form can only express an inner join.
func TestUpdateJoinLeftUnsupportedOnSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	seedWidgetOrders(ctx, t, conn)

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	_, err := UpdateTable(widgets).Join(rel, LeftJoin).Set(Set(widgetQty, int64(1))).Exec(ctx, conn)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestMutateJoinCapabilityGate drives joined UPDATE/DELETE through a
// base-only dialect (no MutateJoinDialect method set) asserting the typed
// dialect.ErrUnsupportedByDialect, never a panic or a silent wrong-SQL
// fallback. Postgres is exercised as the positive path: a joined UPDATE
// renders and runs.
func TestMutateJoinCapabilityGate(t *testing.T) {
	ctx := t.Context()

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	_, err := UpdateTable(widgets).Join(rel, InnerJoin).Set(Set(widgetQty, int64(1))).Exec(ctx, mockExec{dialectName: "mock-nocap"})
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("joined UPDATE on base-only dialect err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	_, err = DeleteFrom(widgets).Join(rel, InnerJoin).Exec(ctx, mockExec{dialectName: "mock-nocap"})
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("joined DELETE on base-only dialect err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	if _, err := UpdateTable(widgets).Join(rel, InnerJoin).Set(Set(widgetQty, int64(1))).Exec(ctx, mockExec{dialectName: "postgres"}); err != nil {
		t.Fatalf("joined UPDATE INNER JOIN on postgres failed: %v (want it to run)", err)
	}
}

// TestMutateWhereDoubleCombines proves Update/Delete Where ANDs a second
// predicate into the first instead of replacing it.
func TestMutateWhereDoubleCombines(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	n, err := UpdateTable(widgets).
		Where(widgetQty.Gt(int64(5))).
		Where(widgetName.Neq("Gamma")).
		Set(Set(widgetName, "x")).
		Exec(ctx, conn)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	if n != 2 {
		t.Fatalf("updated %d rows, want 2 (w1, w2)", n)
	}

	d, err := DeleteFrom(widgets).
		Where(widgetQty.Gt(int64(5))).
		Where(widgetName.Neq("Gamma")).
		Exec(ctx, conn)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	if d != 2 {
		t.Fatalf("deleted %d rows, want 2", d)
	}
}

// TestMutateExecErrorPaths drives exec-layer, resolve, render and
// no-assignment failures through Insert/Update/Delete Exec and
// ExecReturning.
func TestMutateExecErrorPaths(t *testing.T) {
	ctx := t.Context()
	boom := errors.New("boom")

	full := mockExec{dialectName: "mock-mutatefull"}

	if _, err := UpdateTable(widgets).Exec(ctx, full); err == nil {
		t.Fatal("Update.Exec with no assignments succeeded, want an error")
	}

	if err := InsertInto(widgets).Values(Set(widgetID, "x")).Exec(ctx, fakeDB{}); err == nil {
		t.Fatal("Insert.Exec on an unresolvable dialect succeeded, want an error")
	}

	execErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, execErr: boom}

	if err := InsertInto(widgets).Values(Set(widgetID, "x")).Exec(ctx, execErr); !errors.Is(err, boom) {
		t.Fatalf("Insert.Exec err = %v, want errors.Is(err, boom)", err)
	}

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Exec(ctx, execErr); !errors.Is(err, boom) {
		t.Fatalf("Update.Exec err = %v, want errors.Is(err, boom)", err)
	}

	if _, err := DeleteFrom(widgets).Exec(ctx, execErr); !errors.Is(err, boom) {
		t.Fatalf("Delete.Exec err = %v, want errors.Is(err, boom)", err)
	}

	multi := From(widgetOrders)

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Where(widgetID.InSub(multi)).Exec(ctx, full); err == nil {
		t.Fatal("Update.Exec with a multi-column IN subquery succeeded, want a render error")
	}

	if _, err := DeleteFrom(widgets).Where(widgetID.InSub(multi)).Exec(ctx, full); err == nil {
		t.Fatal("Delete.Exec with a multi-column IN subquery succeeded, want a render error")
	}

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Returning().ExecReturning[*widget](ctx, fakeDB{}); err == nil {
		t.Fatal("Update.ExecReturning on an unresolvable dialect succeeded, want an error")
	}

	if _, err := DeleteFrom(widgets).Returning().ExecReturning[*widget](ctx, fakeDB{}); err == nil {
		t.Fatal("Delete.ExecReturning on an unresolvable dialect succeeded, want an error")
	}

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).Where(widgetID.InSub(multi)).Returning().ExecReturning[*widget](ctx, full); err == nil {
		t.Fatal("Update.ExecReturning with a multi-column IN subquery succeeded, want a render error")
	}

	if _, err := DeleteFrom(widgets).Where(widgetID.InSub(multi)).Returning().ExecReturning[*widget](ctx, full); err == nil {
		t.Fatal("Delete.ExecReturning with a multi-column IN subquery succeeded, want a render error")
	}

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).ExecReturning[*widget](ctx, full); err == nil {
		t.Fatal("Update.ExecReturning without Returning() succeeded, want an error")
	}
}
