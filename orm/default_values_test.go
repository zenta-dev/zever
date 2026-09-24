package orm

import (
	"context"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
)

// defaultWidget is the entity for the DEFAULT VALUES fixture table, whose
// columns carry server-side defaults so an INSERT ... DEFAULT VALUES is
// observable.
type defaultWidget struct {
	ID   string
	Name string
}

func (w *defaultWidget) Scan(row Row) error {
	return row.Scan(&w.ID, &w.Name)
}

var (
	defaultWidgets    = NewTable[defaultWidget]("default_widgets", []string{"id", "name"})
	defaultWidgetName = NewColumn[defaultWidget, string]("default_widgets", "name")
)

// newDefaultsDB opens an in-memory sqlite table whose two columns both have
// server-side defaults, so INSERT ... DEFAULT VALUES fills them.
func newDefaultsDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := t.Context()

	conn := openORMTestDB(ctx, t)

	if _, err := conn.Exec(ctx, `CREATE TABLE default_widgets (id text DEFAULT 'generated', name text DEFAULT 'anonymous')`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	return ctx, conn
}

// TestInsertDefaultValuesRoundTrip proves Insert[T].DefaultValues renders
// and runs `INSERT INTO t DEFAULT VALUES`, observable through the
// server-side column defaults.
func TestInsertDefaultValuesRoundTrip(t *testing.T) {
	ctx, conn := newDefaultsDB(t)

	if err := InsertInto(defaultWidgets).DefaultValues().Exec(ctx, conn); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	rows, err := conn.Query(ctx, `SELECT id, name FROM default_widgets`)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var got defaultWidget

	if !rows.Next() {
		t.Fatalf("no row returned")
	}

	if err := rows.Scan(&got.ID, &got.Name); err != nil {
		t.Fatalf("scan: %v", err)
	}

	if got.ID != "generated" || got.Name != "anonymous" {
		t.Fatalf("row = %+v, want the server-side defaults", got)
	}
}

// TestInsertDefaultValuesReturning proves DefaultValues composes with
// Returning/ExecReturning.
func TestInsertDefaultValuesReturning(t *testing.T) {
	ctx, conn := newDefaultsDB(t)

	rows, err := InsertInto(defaultWidgets).DefaultValues().Returning().ExecReturning(ctx, conn)
	if err != nil {
		t.Fatalf("ExecReturning: %v", err)
	}

	if len(rows) != 1 {
		t.Fatalf("got %d returned rows, want 1", len(rows))
	}

	if rows[0].ID != "generated" || rows[0].Name != "anonymous" {
		t.Fatalf("returned row = %+v, want the server-side defaults", rows[0])
	}
}

// TestInsertDefaultValuesRenderedSQL asserts the exact statement text.
func TestInsertDefaultValuesRenderedSQL(t *testing.T) {
	ctx, conn := newDefaultsDB(t)

	rec := &recordingDB{DB: conn}

	if err := InsertInto(defaultWidgets).DefaultValues().Exec(ctx, rec); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	if want := `INSERT INTO "default_widgets" DEFAULT VALUES`; rec.query != want {
		t.Fatalf("query = %q, want %q", rec.query, want)
	}
}

// TestInsertDefaultValuesMutuallyExclusive proves DefaultValues fails
// closed when combined with Values, Columns, or Select rather than silently
// picking a source.
func TestInsertDefaultValuesMutuallyExclusive(t *testing.T) {
	ctx := t.Context()

	withValues := InsertInto(defaultWidgets).DefaultValues().Values(Set(defaultWidgetName, "x"))
	if err := withValues.Exec(ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatalf("DefaultValues + Values succeeded, want an error")
	}

	withColumns := InsertInto(defaultWidgets).Columns(defaultWidgetName.Col()).DefaultValues()
	if err := withColumns.Exec(ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatalf("DefaultValues + Columns succeeded, want an error")
	}

	withSelect := InsertInto(defaultWidgets).DefaultValues().Select(From(defaultWidgets))
	if err := withSelect.Exec(ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatalf("DefaultValues + Select succeeded, want an error")
	}

	// The reverse order -- Values first, then DefaultValues -- must also fail.
	reverse := InsertInto(defaultWidgets).Values(Set(defaultWidgetName, "x")).DefaultValues()
	if err := reverse.Exec(ctx, mockExec{dialectName: "sqlite"}); err == nil {
		t.Fatalf("Values + DefaultValues succeeded, want an error")
	}
}

// TestInsertDefaultValuesWithOnConflictRejected proves DefaultValues cannot
// silently combine with an upsert (an upsert needs a row to conflict on).
func TestInsertDefaultValuesWithOnConflictRejected(t *testing.T) {
	ctx := t.Context()

	err := InsertInto(defaultWidgets).
		DefaultValues().
		OnConflict().
		DoNothing().
		Exec(ctx, mockExec{dialectName: "sqlite"})
	if err == nil {
		t.Fatalf("DefaultValues + OnConflict succeeded, want an error")
	}
}

// TestDefaultValuesErrorMentionsFeature guards the error text carries the
// orm: prefix and names the conflict.
func TestDefaultValuesErrorMentionsFeature(t *testing.T) {
	ctx := t.Context()

	err := InsertInto(defaultWidgets).DefaultValues().
		Values(Set(defaultWidgetName, "x")).
		Exec(ctx, mockExec{dialectName: "sqlite"})
	if err == nil {
		t.Fatalf("expected an error")
	}

	if !strings.Contains(err.Error(), "orm:") {
		t.Fatalf("err = %q, want the orm: package tag", err.Error())
	}
}
