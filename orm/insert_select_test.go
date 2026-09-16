package orm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

// widgetArchive / widgetPairs are second tables of the widget entity used by
// the INSERT ... SELECT round-trips. widgetArchive mirrors widgets' full
// column list (so a SELECT of the entity copies over directly);
// widgetPairs projects just two columns, for the explicit-column mapping
// tests.
var (
	widgetArchive = NewTable[widget]("widget_archive", []string{"id", "name", "quantity", "bio"})
	widgetPairs   = NewTable[widget]("widget_pairs", []string{"id", "name"})
)

// newInsertSelectDB opens an in-memory sqlite database with the widgets
// source table (seeded like newWidgetsDB) plus empty widget_archive and
// widget_pairs target tables.
func newInsertSelectDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := context.Background()

	conn := openORMTestDB(ctx, t)

	for _, stmt := range []string{
		`CREATE TABLE widgets (id text, name text, quantity integer, bio text)`,
		`CREATE TABLE widget_archive (id text UNIQUE, name text, quantity integer, bio text)`,
		`CREATE TABLE widget_pairs (id text, name text)`,
	} {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("create %q: %v", stmt, err)
		}
	}

	rows := []struct {
		id, name string
		qty      int64
		bio      any
	}{
		{"w1", "Alpha", 10, "first"},
		{"w2", "Beta", 20, nil},
		{"w3", "Gamma", 30, "third"},
	}

	for _, r := range rows {
		if _, err := conn.Exec(ctx, `INSERT INTO widgets (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, r.id, r.name, r.qty, r.bio); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	return ctx, conn
}

// archiveByID returns the widget_archive rows keyed by id.
func archiveByID(ctx context.Context, t *testing.T, conn db.DB) map[string]*widget {
	t.Helper()

	rows, err := From(widgetArchive).All(ctx, conn)
	if err != nil {
		t.Fatalf("archive All: %v", err)
	}

	out := make(map[string]*widget, len(rows))
	for _, r := range rows {
		out[r.ID] = r
	}

	return out
}

// pairByID returns the widget_pairs rows as id -> name.
func pairByID(ctx context.Context, t *testing.T, conn db.DB) map[string]string {
	t.Helper()

	rows, err := conn.Query(ctx, `SELECT id, name FROM widget_pairs`)
	if err != nil {
		t.Fatalf("query pairs: %v", err)
	}

	defer func() { _ = rows.Close() }()

	out := map[string]string{}

	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatalf("scan pairs: %v", err)
		}

		out[id] = name
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("pairs rows.Err: %v", err)
	}

	return out
}

// TestInsertSelectInfersColumns proves a bare .Select infers the target
// column list from the SELECT's projection and copies the filtered subset
// (and a NULL bio) into the archive table.
func TestInsertSelectInfersColumns(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	if err := InsertInto(widgetArchive).
		Select(From(widgets).Where(widgetQty.Gt(int64(15)))).
		Exec(ctx, conn); err != nil {
		t.Fatalf("Insert...Select Exec: %v", err)
	}

	got := archiveByID(ctx, t, conn)

	if len(got) != 2 || got["w2"] == nil || got["w3"] == nil {
		t.Fatalf("archive = %v, want exactly w2 and w3 (quantity > 15)", got)
	}

	if _, ok := got["w2"].Bio.Get(); ok {
		t.Fatalf("w2.Bio = %v, want SQL NULL preserved by the SELECT", got["w2"].Bio)
	}

	if bio, ok := got["w3"].Bio.Get(); !ok || bio != "third" {
		t.Fatalf("w3.Bio = (%q, %v), want (\"third\", true)", bio, ok)
	}
}

// TestInsertSelectExplicitColumns proves .Columns + .Select map positionally
// into a two-column target.
func TestInsertSelectExplicitColumns(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	if err := InsertInto(widgetPairs).
		Columns(widgetID.Col(), widgetName.Col()).
		Select(From(widgets).
			Columns(widgetID.Col(), widgetName.Col()).
			Where(widgetQty.Gt(int64(15)))).
		Exec(ctx, conn); err != nil {
		t.Fatalf("Insert...Select Exec: %v", err)
	}

	got := pairByID(ctx, t, conn)
	if len(got) != 2 || got["w2"] != "Beta" || got["w3"] != "Gamma" {
		t.Fatalf("pairs = %v, want {w2:Beta, w3:Gamma}", got)
	}
}

// TestInsertSelectColumnMappingIsPositional proves the INSERT target columns
// drive the mapping: declaring (name, id) while SELECTing (id, name) swaps
// the two values.
func TestInsertSelectColumnMappingIsPositional(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	if err := InsertInto(widgetPairs).
		Columns(widgetName.Col(), widgetID.Col()).
		Select(From(widgets).
			Columns(widgetID.Col(), widgetName.Col()).
			Where(widgetID.Eq("w1"))).
		Exec(ctx, conn); err != nil {
		t.Fatalf("Insert...Select Exec: %v", err)
	}

	got := pairByID(ctx, t, conn)
	if len(got) != 1 {
		t.Fatalf("pairs = %v, want one row", got)
	}

	// Target (name, id) received ("w1", "Alpha"), so id -> name is the swap:
	// the row's id column holds "Alpha" and its name holds "w1".
	if got["Alpha"] != "w1" {
		t.Fatalf("pairs = %v, want {Alpha:w1} (positional swap)", got)
	}
}

// TestInsertSelectRendersNoValuesAndKeepsSelectClause proves the rendered
// statement is `INSERT ... SELECT ...` (no VALUES keyword) and reuses the
// Query's WHERE clause.
func TestInsertSelectRendersNoValuesAndKeepsSelectClause(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	rec := &recordingDB{DB: conn}

	if err := InsertInto(widgetArchive).
		Select(From(widgets).Where(widgetQty.Gt(int64(15)))).
		Exec(ctx, rec); err != nil {
		t.Fatalf("Insert...Select Exec: %v", err)
	}

	if strings.Contains(rec.query, "VALUES") {
		t.Fatalf("query = %q, want no VALUES keyword", rec.query)
	}

	want := `INSERT INTO "widget_archive" ("id", "name", "quantity", "bio") SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE "quantity" > ?`
	if rec.query != want {
		t.Fatalf("query = %q, want %q", rec.query, want)
	}
}

// TestInsertSelectOnConflictDoUpdate proves INSERT ... SELECT composes with
// ON CONFLICT: an existing archive row is updated, a new one inserted.
func TestInsertSelectOnConflictDoUpdate(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	if _, err := conn.Exec(ctx, `INSERT INTO widget_archive (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, "w2", "Old", int64(1), nil); err != nil {
		t.Fatalf("seed archive: %v", err)
	}

	if err := InsertInto(widgetArchive).
		OnConflict(widgetID.Col()).
		DoUpdate(Set(widgetName, "Merged")).
		Select(From(widgets).Where(widgetQty.Gt(int64(15)))).
		Exec(ctx, conn); err != nil {
		t.Fatalf("Insert...Select OnConflict Exec: %v", err)
	}

	got := archiveByID(ctx, t, conn)

	if got["w2"] == nil || got["w2"].Name != "Merged" {
		t.Fatalf("w2 after upsert-from-select = %+v, want name Merged", got["w2"])
	}

	if got["w3"] == nil || got["w3"].Name != "Gamma" {
		t.Fatalf("w3 after upsert-from-select = %+v, want inserted Gamma", got["w3"])
	}
}

// TestInsertSelectOnConflictDoNothing proves the DO NOTHING conflict action
// leaves the conflicting archive row untouched while inserting the rest.
func TestInsertSelectOnConflictDoNothing(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	if _, err := conn.Exec(ctx, `INSERT INTO widget_archive (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, "w2", "Old", int64(1), nil); err != nil {
		t.Fatalf("seed archive: %v", err)
	}

	if err := InsertInto(widgetArchive).
		Select(From(widgets).Where(widgetQty.Gt(int64(15)))).
		OnConflict(widgetID.Col()).
		DoNothing().
		Exec(ctx, conn); err != nil {
		t.Fatalf("Insert...Select DoNothing Exec: %v", err)
	}

	got := archiveByID(ctx, t, conn)

	if got["w2"] == nil || got["w2"].Name != "Old" {
		t.Fatalf("w2 after DO NOTHING = %+v, want untouched Old", got["w2"])
	}

	if got["w3"] == nil {
		t.Fatalf("w3 missing after DO NOTHING; want inserted")
	}
}

// TestInsertSelectReturning proves the SELECT-backed insert composes with
// RETURNING and hands back the inserted rows.
func TestInsertSelectReturning(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	got, err := InsertInto(widgetArchive).
		Select(From(widgets).Where(widgetQty.Gt(int64(15)))).
		Returning().
		ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("ExecReturning returned %d rows, want 2", len(got))
	}

	ids := map[string]bool{got[0].ID: true, got[1].ID: true}
	if !ids["w2"] || !ids["w3"] {
		t.Fatalf("returned ids = %v, want w2 and w3", ids)
	}
}

// TestInsertSelectValuesMutuallyExclusive proves .Values followed by .Select
// is rejected instead of silently picking one source.
func TestInsertSelectValuesMutuallyExclusive(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	err := InsertInto(widgetArchive).
		Values(Set(widgetID, "x"), Set(widgetName, "X"), Set(widgetQty, int64(1)), widgetBio.SetNull()).
		Select(From(widgets)).
		Exec(ctx, conn)
	if err == nil {
		t.Fatalf("Values + Select succeeded, want a typed error")
	}
}

// TestInsertSelectColumnCountMismatch proves a target column count that
// disagrees with the SELECT projection is rejected at render time.
func TestInsertSelectColumnCountMismatch(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	err := InsertInto(widgetArchive).
		Columns(widgetID.Col()).
		Select(From(widgets).Columns(widgetID.Col(), widgetName.Col())).
		Exec(ctx, conn)
	if err == nil {
		t.Fatalf("mismatched column count succeeded, want a typed error")
	}
}

// TestInsertSelectNoProjectionGuard proves a SELECT whose projection cannot
// be trusted -- here a table with no columns and no explicit Columns() --
// is rejected rather than rendering a zero-column INSERT.
func TestInsertSelectNoProjectionGuard(t *testing.T) {
	ctx, conn := newInsertSelectDB(t)

	empty := NewTable[widget]("empty", nil)

	err := InsertInto(widgetArchive).Select(From(empty)).Exec(ctx, conn)
	if err == nil {
		t.Fatalf("Select over a zero-column projection succeeded, want a typed error")
	}
}

// TestInsertSelectReturningGate proves the SELECT-backed insert does not
// bypass the RETURNING capability gate: a dialect without RETURNING is
// rejected with the typed dialect.ErrUnsupportedByDialect.
func TestInsertSelectReturningGate(t *testing.T) {
	_, err := InsertInto(widgetArchive).
		Select(From(widgets).Where(widgetQty.Gt(int64(15)))).
		Returning().
		ExecReturning(context.Background(), mockExec{dialectName: "mock-nocap"})
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestInsertSelectColumnsCopyOnWrite proves .Columns keeps the branch-safety
// discipline: two inserts branched from one base never share a column slice.
func TestInsertSelectColumnsCopyOnWrite(t *testing.T) {
	base := InsertInto(widgetArchive)

	a := base.Columns(widgetID.Col(), widgetName.Col())
	b := base.Columns(widgetID.Col(), widgetName.Col(), widgetQty.Col())

	if len(base.columns) != 0 {
		t.Fatalf("base.columns mutated by branching: %v", base.columns)
	}

	if len(a.columns) != 2 || len(b.columns) != 3 {
		t.Fatalf("branch column lengths = (%d, %d), want (2, 3)", len(a.columns), len(b.columns))
	}
}

// TestInsertSelectRenderErrorPaths proves SELECT-backed conflict validation
// fails closed: DO UPDATE without sets, an unsupported conflict WHERE, and
// a gated dialect each surface before any SQL is issued.
func TestInsertSelectRenderErrorPaths(t *testing.T) {
	ctx := context.Background()

	noSets := InsertInto(widgetArchive).
		OnConflict(widgetID.Col()).
		DoUpdate().
		Select(From(widgets).Where(widgetQty.Gt(int64(15))))

	if err := noSets.Exec(ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatal("Exec with DO UPDATE and no sets succeeded, want an error")
	}

	whereGated := InsertInto(widgetArchive).
		OnConflict(widgetID.Col()).
		Where(widgetQty.Eq(int64(1))).
		DoNothing().
		Select(From(widgets).Where(widgetQty.Gt(int64(15))))

	if err := whereGated.Exec(ctx, mockExec{dialectName: "mock-nocap"}); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("Exec err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestInsertSelectSourceRenderError proves a SELECT source that cannot
// render fails the INSERT before any SQL is issued.
func TestInsertSelectSourceRenderError(t *testing.T) {
	multi := From(widgetOrders)

	err := InsertInto(widgetArchive).
		Select(From(widgets).Where(widgetID.InSub(multi))).
		Exec(context.Background(), mockExec{dialectName: "sqlite"})
	if err == nil {
		t.Fatal("Exec with a multi-column IN subquery source succeeded, want a render error")
	}
}
