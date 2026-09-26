// Command app is the bulk_upsert example: a widget inventory table
// (schema/bulk_upsert.zen, codegen'd by the zenorm backend into
// generated/zenorm/orm/gen/app) written in bulk via orm.Insert...Values(...)
// .Values(...) (one multi-row INSERT), then upserted with
// OnConflict(id).DoUpdate(...).Returning() -- the conflicting row is
// updated in place and every written row comes back through the codegen'd
// Scan method, all in a single statement.
//
// A partial unique index plus OnConflict(...).Where(...) then shows the
// conflict-target predicate: only rows covered by the partial index
// arbitrate the upsert, so a colliding name outside the index inserts a
// second row instead of updating.
//
// The legacy ORM's CreateUsers batched inserts by hand (a fixed 5000-row
// chunk loop over engine.RenderInsertMany) and had no conflict handling or
// RETURNING at all; orm folds all three -- multi-row, upsert, RETURNING --
// into the Insert builder.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	gen "github.com/zenta-dev/zever/examples/bulk_upsert/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

func main() {
	ctx := context.Background()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		die(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := seed(ctx, conn); err != nil {
		die(err)
	}

	if err := bulkInsertNew(ctx, conn); err != nil {
		die(err)
	}

	if _, err := bulkUpsert(ctx, conn); err != nil {
		die(err)
	}

	if _, err := upsertDoNothing(ctx, conn); err != nil {
		die(err)
	}

	if _, _, err := conditionalUpsert(ctx, conn); err != nil {
		die(err)
	}

	if err := verifyFinalInventory(ctx, conn); err != nil {
		die(err)
	}
}

// seed creates the widgets table (UNIQUE on id, the conflict target) and
// two starting widgets.
func seed(ctx context.Context, conn db.DB) error {
	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id text UNIQUE, name text, price_cents integer, stock integer)`); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	for _, w := range []gen.Widget{
		{ID: "w1", Name: "gizmo", PriceCents: 1200, Stock: 10},
		{ID: "w2", Name: "doohickey", PriceCents: 450, Stock: 40},
	} {
		insert := orm.InsertInto(gen.Widgets).Values(
			orm.Set(gen.WidgetCols.ID, w.ID),
			orm.Set(gen.WidgetCols.Name, w.Name),
			orm.Set(gen.WidgetCols.PriceCents, w.PriceCents),
			orm.Set(gen.WidgetCols.Stock, w.Stock),
		)

		if err := insert.Exec(ctx, conn); err != nil {
			return fmt.Errorf("insert %s: %w", w.ID, err)
		}
	}

	return nil
}

// widgetValues is the per-row assignment list every demo inserts. The FIRST
// Values call on a chain fixes the column order for all rows of that
// INSERT, so every row supplies the same four columns in the same order.
func widgetValues(w gen.Widget) []orm.Assignment[gen.Widget] {
	return []orm.Assignment[gen.Widget]{
		orm.Set(gen.WidgetCols.ID, w.ID),
		orm.Set(gen.WidgetCols.Name, w.Name),
		orm.Set(gen.WidgetCols.PriceCents, w.PriceCents),
		orm.Set(gen.WidgetCols.Stock, w.Stock),
	}
}

// bulkInsertNew inserts three brand-new widgets in ONE multi-row INSERT
// (chained Values calls). Plain INSERTs do not render RETURNING -- it only
// takes effect on the OnConflict path (see bulkUpsert) -- so the rows are
// read back with an ordinary query instead.
func bulkInsertNew(ctx context.Context, conn db.DB) error {
	insert := orm.InsertInto(gen.Widgets).
		Values(widgetValues(gen.Widget{ID: "w3", Name: "sprocket", PriceCents: 990, Stock: 25})...).
		Values(widgetValues(gen.Widget{ID: "w4", Name: "cog", PriceCents: 310, Stock: 60})...).
		Values(widgetValues(gen.Widget{ID: "w5", Name: "ratchet", PriceCents: 2100, Stock: 5})...)

	if err := insert.Exec(ctx, conn); err != nil {
		return fmt.Errorf("bulk insert: %w", err)
	}

	rows, err := orm.From(gen.Widgets).
		Where(orm.Or(
			gen.WidgetCols.ID.Eq("w3"),
			gen.WidgetCols.ID.Eq("w4"),
			gen.WidgetCols.ID.Eq("w5"),
		)).
		OrderBy(gen.WidgetCols.ID.Asc()).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("read back bulk insert: %w", err)
	}

	fmt.Println("bulk INSERT of 3 new rows (one statement, read back):")

	for _, w := range rows {
		fmt.Printf("  %s %-10s %4d.%02d stock=%d\n", w.ID, w.Name, w.PriceCents/100, w.PriceCents%100, w.Stock)
	}

	return nil
}

// bulkUpsert upserts four rows in one statement: w1 and w2 already exist
// (DO UPDATE applies the batch's assignments to the conflicting row), w6
// and w7 are new (plain insert). RETURNING hands every written row back.
//
// Note the DO UPDATE assignments are LITERALS applied uniformly to every
// conflicting row -- orm has no Postgres EXCLUDED pseudo-row reference --
// so this is a "reconcile the whole batch" shape: each colliding row's
// price and stock are reset to this batch's known-good values.
func bulkUpsert(ctx context.Context, conn db.DB) ([]*gen.Widget, error) {
	rows, err := orm.InsertInto(gen.Widgets).
		Values(widgetValues(gen.Widget{ID: "w1", Name: "gizmo", PriceCents: 1500, Stock: 12})...).
		Values(widgetValues(gen.Widget{ID: "w2", Name: "doohickey", PriceCents: 490, Stock: 55})...).
		Values(widgetValues(gen.Widget{ID: "w6", Name: "thimble", PriceCents: 180, Stock: 200})...).
		Values(widgetValues(gen.Widget{ID: "w7", Name: "widget-7", PriceCents: 800, Stock: 8})...).
		OnConflict(gen.WidgetCols.ID.Col()).
		DoUpdate(
			orm.Set(gen.WidgetCols.PriceCents, int64(1234)),
			orm.Set(gen.WidgetCols.Stock, int64(77)),
		).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("bulk upsert: %w", err)
	}

	fmt.Println("bulk upsert of 4 rows (2 existing updated, 2 inserted), RETURNING all of them:")

	for _, w := range rows {
		fmt.Printf("  %s %-10s %4d.%02d stock=%d\n", w.ID, w.Name, w.PriceCents/100, w.PriceCents%100, w.Stock)
	}

	return rows, nil
}

// upsertDoNothing re-inserts w1 with a bogus price; the ON CONFLICT DO
// NOTHING branch fires, so no RETURNING row comes back and w1's real values
// are untouched.
func upsertDoNothing(ctx context.Context, conn db.DB) (int, error) {
	rows, err := orm.InsertInto(gen.Widgets).
		Values(widgetValues(gen.Widget{ID: "w1", Name: "should-not-stick", PriceCents: 1, Stock: 1})...).
		OnConflict(gen.WidgetCols.ID.Col()).
		DoNothing().
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		return 0, fmt.Errorf("upsert do nothing: %w", err)
	}

	fmt.Printf("re-insert of w1 with ON CONFLICT DO NOTHING: %d RETURNING rows (the conflict produced none)\n", len(rows))

	return len(rows), nil
}

// conditionalUpsert demonstrates the ON CONFLICT target WHERE predicate:
// a partial unique index covers only stocked widgets, so the predicate
// picks that index as the arbiter. A name colliding inside the index
// updates the existing row; a name colliding outside it inserts a new row.
func conditionalUpsert(ctx context.Context, conn db.DB) ([]*gen.Widget, []*gen.Widget, error) {
	if _, err := conn.Exec(ctx, `CREATE UNIQUE INDEX widgets_name_active ON widgets (name) WHERE stock > 0`); err != nil {
		return nil, nil, fmt.Errorf("create partial index: %w", err)
	}

	for _, w := range []gen.Widget{
		{ID: "w8", Name: "gadget-x", PriceCents: 500, Stock: 10},
		{ID: "w10", Name: "dormant", PriceCents: 600, Stock: 0},
	} {
		if err := orm.InsertInto(gen.Widgets).Values(widgetValues(w)...).Exec(ctx, conn); err != nil {
			return nil, nil, fmt.Errorf("insert %s: %w", w.ID, err)
		}
	}

	// "gadget-x" is in the partial index (w8 has stock 10), so this
	// collides and updates w8 in place; the proposed id w9 never lands.
	updated, err := orm.InsertInto(gen.Widgets).
		Values(widgetValues(gen.Widget{ID: "w9", Name: "gadget-x", PriceCents: 555, Stock: 11})...).
		OnConflict(gen.WidgetCols.Name.Col()).
		Where(orm.UnsafeRaw[gen.Widget]("stock > 0")).
		DoUpdate(
			orm.Set(gen.WidgetCols.PriceCents, int64(777)),
			orm.Set(gen.WidgetCols.Stock, int64(33)),
		).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		return nil, nil, fmt.Errorf("conditional upsert (indexed name): %w", err)
	}

	fmt.Println(`upsert of "gadget-x" with ON CONFLICT (name) WHERE stock > 0 (matched the partial index, updated w8):`)

	for _, w := range updated {
		fmt.Printf("  %s %-10s %4d.%02d stock=%d\n", w.ID, w.Name, w.PriceCents/100, w.PriceCents%100, w.Stock)
	}

	// "dormant" is NOT in the partial index (w10 has stock 0), so there is
	// nothing to collide with: w11 inserts as a second "dormant" row.
	inserted, err := orm.InsertInto(gen.Widgets).
		Values(widgetValues(gen.Widget{ID: "w11", Name: "dormant", PriceCents: 100, Stock: 1})...).
		OnConflict(gen.WidgetCols.Name.Col()).
		Where(orm.UnsafeRaw[gen.Widget]("stock > 0")).
		DoUpdate(
			orm.Set(gen.WidgetCols.PriceCents, int64(777)),
			orm.Set(gen.WidgetCols.Stock, int64(33)),
		).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		return nil, nil, fmt.Errorf("conditional upsert (unindexed name): %w", err)
	}

	fmt.Println(`upsert of "dormant" with ON CONFLICT (name) WHERE stock > 0 (outside the partial index, inserted w11):`)

	for _, w := range inserted {
		fmt.Printf("  %s %-10s %4d.%02d stock=%d\n", w.ID, w.Name, w.PriceCents/100, w.PriceCents%100, w.Stock)
	}

	return updated, inserted, nil
}

// verifyFinalInventory lists every widget after all demos, proving the
// DoNothing branch left w1 alone while the DoUpdate branch rewrote it, and
// the partial-index upsert updated w8 but duplicated "dormant".
func verifyFinalInventory(ctx context.Context, conn db.DB) error {
	rows, err := orm.From(gen.Widgets).OrderBy(gen.WidgetCols.ID.Asc()).All(ctx, conn)
	if err != nil {
		return fmt.Errorf("final inventory: %w", err)
	}

	fmt.Printf("final inventory (%d widgets):\n", len(rows))

	for _, w := range rows {
		fmt.Printf("  %s %-10s %4d.%02d stock=%d\n", w.ID, w.Name, w.PriceCents/100, w.PriceCents%100, w.Stock)
	}

	return nil
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
