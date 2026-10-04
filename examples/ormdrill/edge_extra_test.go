package ormdrill

import (
	"path/filepath"
	"testing"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm"
)

// TestPreloadEmptyDatabase pins the empty-collection boundary: preloading an
// empty shop yields no parents and no error.
func TestPreloadEmptyDatabase(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	conn, err := sqlite.New(db.Options{Path: filepath.Join(t.TempDir(), "empty.db")})
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	for _, stmt := range []string{
		`CREATE TABLE widgets (id text, name text, price_cents integer, created_at text, note text)`,
		`CREATE TABLE orders (id text, widget_id text, amount_cents integer, created_at text)`,
	} {
		if _, execErr := conn.Exec(ctx, stmt); execErr != nil {
			t.Fatalf("create: %v", execErr)
		}
	}

	out, err := orm.Preload(ctx, conn,
		orm.From(Widgets).OrderBy(WidgetCols.ID.Asc()),
		OrderCols.WidgetID,
		orm.From(Orders).OrderBy(OrderCols.CreatedAt.Asc()),
		func(w *Widget) string { return w.ID },
		func(o *Order) string { return o.WidgetID },
	)
	if err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("parents = %d, want 0", len(out))
	}
}

// TestWidgetsAllEmpty pins the typed table scan over an empty table.
func TestWidgetsAllEmpty(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	conn, err := sqlite.New(db.Options{Path: filepath.Join(t.TempDir(), "empty.db")})
	if err != nil {
		t.Fatalf("sqlite.New: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	if _, execErr := conn.Exec(ctx, `CREATE TABLE widgets (id text, name text, price_cents integer, created_at text, note text)`); execErr != nil {
		t.Fatalf("create: %v", execErr)
	}

	rows, err := orm.From(Widgets).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}
