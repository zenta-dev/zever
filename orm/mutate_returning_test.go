package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
)

// TestUpdateReturningRoundTrip proves Update.Returning returns the
// POST-write row values: the SET column carries its new value, an un-SET
// column its pre-update value (RETURNING reports the row as written, not a
// pre-write snapshot). It also asserts the rendered SQL.
func TestUpdateReturningRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rec := &recordingDB{DB: conn}

	got, err := UpdateTable(widgets).
		Where(widgetID.Eq("w1")).
		Set(Set(widgetName, "Renamed")).
		Returning().
		ExecReturning(ctx, rec)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	wantSQL := `UPDATE "widgets" SET "name" = ? WHERE "id" = ? RETURNING "id", "name", "quantity", "bio"`
	if rec.query != wantSQL {
		t.Fatalf("query = %q, want %q", rec.query, wantSQL)
	}

	if len(got) != 1 {
		t.Fatalf("ExecReturning returned %d rows, want 1", len(got))
	}

	if got[0].ID != "w1" || got[0].Name != "Renamed" {
		t.Fatalf("returned row = %+v, want w1/Renamed (name was SET)", got[0])
	}

	if got[0].Quantity != 10 {
		t.Fatalf("returned row.Quantity = %d, want 10 (quantity not in SET list, must be pre-update value)", got[0].Quantity)
	}

	bioVal, bioOK := got[0].Bio.Get()
	if !bioOK || bioVal != "first" {
		t.Fatalf("returned row.Bio = (%q, %v), want (\"first\", true) -- bio not in SET list, must be pre-update value", bioVal, bioOK)
	}
}

// TestUpdateReturningNoMatch proves an UPDATE that matches no rows returns
// no RETURNING rows.
func TestUpdateReturningNoMatch(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	got, err := UpdateTable(widgets).
		Where(widgetID.Eq("nope")).
		Set(Set(widgetName, "x")).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("ExecReturning returned %d rows, want 0 for a no-match UPDATE", len(got))
	}
}

// TestUpdateReturningExplicitColumns proves an explicit column list renders
// (RETURNING each named column, in caller order) and, when kept aligned with
// T's codegen'd Scan, round-trips into T just like the no-arg form.
func TestUpdateReturningExplicitColumns(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rec := &recordingDB{DB: conn}

	got, err := UpdateTable(widgets).
		Where(widgetID.Eq("w2")).
		Set(Set(widgetName, "Beta2"), Set(widgetQty, int64(21))).
		Returning(widgetID.Col(), widgetName.Col(), widgetQty.Col(), widgetBio.Col()).
		ExecReturning(ctx, rec)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	if len(got) != 1 || got[0].ID != "w2" || got[0].Name != "Beta2" || got[0].Quantity != 21 {
		t.Fatalf("returned row = %+v, want w2/Beta2/21", got)
	}

	if got[0].Bio.IsSome() {
		t.Fatalf("returned row.Bio.IsSome() = true, want false (w2 bio is NULL)")
	}

	wantSQL := `UPDATE "widgets" SET "name" = ?, "quantity" = ? WHERE "id" = ? RETURNING "id", "name", "quantity", "bio"`
	if rec.query != wantSQL {
		t.Fatalf("query = %q, want %q (explicit column list renders each named column)", rec.query, wantSQL)
	}
}

// TestDeleteReturningRoundTrip proves Delete.Returning returns the deleted
// rows' full column values and the rows are gone.
func TestDeleteReturningRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rec := &recordingDB{DB: conn}

	got, err := DeleteFrom(widgets).
		Where(widgetID.Eq("w1")).
		Returning().
		ExecReturning(ctx, rec)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	wantSQL := `DELETE FROM "widgets" WHERE "id" = ? RETURNING "id", "name", "quantity", "bio"`
	if rec.query != wantSQL {
		t.Fatalf("query = %q, want %q", rec.query, wantSQL)
	}

	if len(got) != 1 {
		t.Fatalf("ExecReturning returned %d rows, want 1", len(got))
	}

	if got[0].ID != "w1" || got[0].Name != "Alpha" || got[0].Quantity != 10 {
		t.Fatalf("returned row = %+v, want deleted w1/Alpha/10", got[0])
	}

	bioVal, bioOK := got[0].Bio.Get()
	if !bioOK || bioVal != "first" {
		t.Fatalf("returned row.Bio = (%q, %v), want (\"first\", true)", bioVal, bioOK)
	}

	exists, err := From(widgets).Where(widgetID.Eq("w1")).Exists(ctx, conn)
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}

	if exists {
		t.Fatalf("w1 still exists after ExecReturning delete")
	}
}

// TestDeleteReturningManyRows proves a multi-row DELETE returns one row per
// deleted row, carrying each row's OWN values.
func TestDeleteReturningManyRows(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	got, err := DeleteFrom(widgets).
		Where(widgetID.In("w1", "w2")).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("ExecReturning returned %d rows, want 2", len(got))
	}

	ids := map[string]bool{}
	for _, row := range got {
		ids[row.ID] = true
	}

	if !ids["w1"] || !ids["w2"] {
		t.Fatalf("returned ids = %v, want both w1 and w2", ids)
	}
}

// TestMutateExecRejectsReturning proves Update/Delete.Exec refuse to
// silently drop a requested RETURNING clause, mirroring Insert.Exec.
func TestMutateExecRejectsReturning(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	if _, err := UpdateTable(widgets).
		Where(widgetID.Eq("w1")).
		Set(Set(widgetName, "x")).
		Returning().
		Exec(ctx, conn); err == nil {
		t.Fatalf("Update.Exec after Returning() succeeded, want an error directing to ExecReturning")
	}

	if _, err := DeleteFrom(widgets).
		Where(widgetID.Eq("w1")).
		Returning().
		Exec(ctx, conn); err == nil {
		t.Fatalf("Delete.Exec after Returning() succeeded, want an error directing to ExecReturning")
	}
}

// TestMutateExecReturningRequiresReturning proves ExecReturning refuses to
// run when no RETURNING columns were requested.
func TestMutateExecReturningRequiresReturning(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	if _, err := UpdateTable(widgets).
		Where(widgetID.Eq("w1")).
		Set(Set(widgetName, "x")).
		ExecReturning(ctx, conn); err == nil {
		t.Fatalf("Update.ExecReturning without Returning() succeeded, want an error")
	}

	if _, err := DeleteFrom(widgets).
		Where(widgetID.Eq("w1")).
		ExecReturning(ctx, conn); err == nil {
		t.Fatalf("Delete.ExecReturning without Returning() succeeded, want an error")
	}
}

// TestMutateReturningCapabilityGate drives Update/Delete ExecReturning
// through a dialect that cannot support RETURNING -- a base-only dialect (no
// ReturningDialect method set) -- asserting the typed
// dialect.ErrUnsupportedByDialect. Postgres is the positive path: the gate
// passes and the mocked empty result set yields no rows.
func TestMutateReturningCapabilityGate(t *testing.T) {
	ctx := context.Background()

	upd := UpdateTable(widgets).
		Where(widgetID.Eq("w1")).
		Set(Set(widgetName, "x")).
		Returning()
	del := DeleteFrom(widgets).
		Where(widgetID.Eq("w1")).
		Returning()

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"Update/base-only", func() error {
			_, err := upd.ExecReturning(ctx, mockExec{dialectName: "mock-nocap"})

			return err
		}},
		{"Delete/base-only", func() error {
			_, err := del.ExecReturning(ctx, mockExec{dialectName: "mock-nocap"})

			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
				t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
			}
		})
	}

	// Postgres supports RETURNING on UPDATE/DELETE: the gate passes and the
	// empty result set scans to zero rows.
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"Update/postgres", func() error {
			_, err := upd.ExecReturning(ctx, mockExec{dialectName: "postgres"})

			return err
		}},
		{"Delete/postgres", func() error {
			_, err := del.ExecReturning(ctx, mockExec{dialectName: "postgres"})

			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err != nil {
				t.Fatalf("err = %v, want nil (postgres supports RETURNING)", err)
			}
		})
	}
}

// TestMutateReturningDoesNotShareBackingArray proves Returning's column
// list is a fresh slice for Update and Delete, so two branches from one
// base cannot corrupt each other.
func TestMutateReturningDoesNotShareBackingArray(t *testing.T) {
	baseU := UpdateTable(widgets).Set(Set(widgetName, "x"))
	branchU := baseU.Returning(widgetID.Col())

	if len(baseU.returning) != 0 {
		t.Fatalf("baseU.returning mutated by branching: %v", baseU.returning)
	}

	if len(branchU.returning) != 1 || branchU.returning[0] != "id" {
		t.Fatalf("branchU.returning = %v, want [id]", branchU.returning)
	}

	baseD := DeleteFrom(widgets)
	branchD := baseD.Returning(widgetID.Col(), widgetName.Col())

	if len(baseD.returning) != 0 {
		t.Fatalf("baseD.returning mutated by branching: %v", baseD.returning)
	}

	if len(branchD.returning) != 2 {
		t.Fatalf("branchD.returning = %v, want two columns", branchD.returning)
	}
}

// TestUpdateJoinReturningRoundTrip proves a joined UPDATE...FROM with
// RETURNING renders and runs against real SQLite (the PR #390 surface):
// w1/w2 match the join and come back in the RETURNING result, w3 does not.
// It also asserts the combined SQL, proving the RETURNING clause stays
// correct after the FROM clause and the placeholder numbering is unchanged
// by it.
func TestUpdateJoinReturningRoundTrip(t *testing.T) {
	ctx, conn := newWidgetsDB(t)
	seedWidgetOrders(ctx, t, conn)

	rec := &recordingDB{DB: conn}

	rel := NewRelation[widget, widgetOrder]("id", "widget_id", widgetOrders)

	got, err := UpdateTable(widgets).
		Join(rel, InnerJoin).
		Where(widgetName.Eq("Alpha")).
		Set(Set(widgetQty, int64(999))).
		Returning().
		ExecReturning(ctx, rec)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	wantSQL := `UPDATE "widgets" SET "quantity" = ? FROM "widget_orders" WHERE "widgets"."id" = "widget_orders"."widget_id" AND "widgets"."name" = ? RETURNING "id", "name", "quantity", "bio"`
	if rec.query != wantSQL {
		t.Fatalf("query = %q, want %q", rec.query, wantSQL)
	}

	if len(got) != 1 {
		t.Fatalf("ExecReturning returned %d rows, want 1 (only w1 matches name=Alpha and has an order)", len(got))
	}

	if got[0].ID != "w1" || got[0].Quantity != 999 {
		t.Fatalf("returned row = %+v, want w1 with updated quantity 999", got[0])
	}

	if got[0].Name != "Alpha" {
		t.Fatalf("returned row.Name = %q, want Alpha (name untouched by the join update)", got[0].Name)
	}
}

// TestMutateReturningValidationEdges proves ExecReturning rejects a
// missing RETURNING clause, a missing assignment list, and a gated
// statement before any SQL is issued.
func TestMutateReturningValidationEdges(t *testing.T) {
	ctx := context.Background()

	if _, err := UpdateTable(widgets).Set(Set(widgetName, "x")).ExecReturning[*widget](ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatal("Update.ExecReturning without Returning() succeeded, want an error")
	}

	if _, err := UpdateTable(widgets).Returning().ExecReturning[*widget](ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatal("Update.ExecReturning without Set succeeded, want an error")
	}

	if _, err := DeleteFrom(widgets).Returning().OrderBy(widgetID.Asc()).Limit(1).ExecReturning[*widget](ctx, mockExec{dialectName: "sqlite"}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Delete.ExecReturning order-limit err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}
