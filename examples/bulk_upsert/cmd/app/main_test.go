package main

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	gen "github.com/zenta-dev/zever/examples/bulk_upsert/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

func openTestDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := context.Background()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	if err := seed(ctx, conn); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return ctx, conn
}

func testWidgetByID(ctx context.Context, t *testing.T, conn db.DB, id string) gen.Widget {
	t.Helper()

	w, ok, err := orm.From(gen.Widgets).Where(gen.WidgetCols.ID.Eq(id)).First(ctx, conn)
	if err != nil {
		t.Fatalf("lookup %s: %v", id, err)
	}

	if !ok {
		t.Fatalf("widget %s not found", id)
	}

	return *w
}

func assertWidget(t *testing.T, w gen.Widget, price, stock int64) {
	t.Helper()

	if w.PriceCents != price || w.Stock != stock {
		t.Fatalf("%s = price %d stock %d, want price %d stock %d", w.ID, w.PriceCents, w.Stock, price, stock)
	}
}

// TestBulkUpsertReturningRows checks the same four RETURNING rows the demo
// prints: the two pre-seeded widgets reconciled to the batch values and the
// two brand-new inserts.
func TestBulkUpsertReturningRows(t *testing.T) {
	ctx, conn := openTestDB(t)

	if err := bulkInsertNew(ctx, conn); err != nil {
		t.Fatalf("bulk insert: %v", err)
	}

	rows, err := bulkUpsert(ctx, conn)
	if err != nil {
		t.Fatalf("bulk upsert: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("RETURNING rows = %d, want 4", len(rows))
	}

	got := make(map[string]gen.Widget, len(rows))
	for _, w := range rows {
		got[w.ID] = *w
	}

	for id, want := range map[string][2]int64{
		"w1": {1234, 77},
		"w2": {1234, 77},
		"w6": {180, 200},
		"w7": {800, 8},
	} {
		w, ok := got[id]
		if !ok {
			t.Fatalf("RETURNING rows missing %s: %v", id, got)
		}

		assertWidget(t, w, want[0], want[1])
	}
}

// TestUpsertDoNothingLeavesRowUntouched re-inserts w1 under DO NOTHING and
// proves no RETURNING row comes back and w1 keeps its upserted values.
func TestUpsertDoNothingLeavesRowUntouched(t *testing.T) {
	ctx, conn := openTestDB(t)

	if err := bulkInsertNew(ctx, conn); err != nil {
		t.Fatalf("bulk insert: %v", err)
	}

	if _, err := bulkUpsert(ctx, conn); err != nil {
		t.Fatalf("bulk upsert: %v", err)
	}

	n, err := upsertDoNothing(ctx, conn)
	if err != nil {
		t.Fatalf("upsert do nothing: %v", err)
	}

	if n != 0 {
		t.Fatalf("RETURNING rows = %d, want 0", n)
	}

	assertWidget(t, testWidgetByID(ctx, t, conn, "w1"), 1234, 77)
}

// TestConditionalUpsertPartialIndex covers the ON CONFLICT target WHERE
// predicate: a name inside the partial index updates the existing row in
// place, while a name outside it inserts a second row.
func TestConditionalUpsertPartialIndex(t *testing.T) {
	ctx, conn := openTestDB(t)

	if err := bulkInsertNew(ctx, conn); err != nil {
		t.Fatalf("bulk insert: %v", err)
	}

	if _, err := bulkUpsert(ctx, conn); err != nil {
		t.Fatalf("bulk upsert: %v", err)
	}

	updated, inserted, err := conditionalUpsert(ctx, conn)
	if err != nil {
		t.Fatalf("conditional upsert: %v", err)
	}

	if len(updated) != 1 || updated[0].ID != "w8" {
		t.Fatalf("indexed upsert RETURNING = %v, want exactly [w8]", updated)
	}

	assertWidget(t, *updated[0], 777, 33)

	if len(inserted) != 1 || inserted[0].ID != "w11" {
		t.Fatalf("unindexed upsert RETURNING = %v, want exactly [w11]", inserted)
	}

	assertWidget(t, *inserted[0], 100, 1)

	// w10 sits outside the partial index (stock 0) and is untouched.
	assertWidget(t, testWidgetByID(ctx, t, conn, "w10"), 600, 0)

	rows, err := orm.From(gen.Widgets).OrderBy(gen.WidgetCols.ID.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("final inventory: %v", err)
	}

	if len(rows) != 10 {
		t.Fatalf("final inventory = %d widgets, want 10", len(rows))
	}
}
