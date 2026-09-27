package orm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
)

// widget is a small fixture entity shared by the query, examples and
// streaming tests, mirroring the shape schema codegen generates: a plain
// struct, a package-level Table var, one Column per field, and a
// pointer-receiver Scan method reading columns positionally.
type widget struct {
	ID       string
	Name     string
	Quantity int64
	Bio      Option[string]
}

func (w *widget) Scan(row Row) error {
	return row.Scan(&w.ID, &w.Name, &w.Quantity, &w.Bio)
}

var (
	widgets    = NewTable[widget]("widgets", []string{"id", "name", "quantity", "bio"})
	widgetID   = NewColumn[widget, string]("widgets", "id")
	widgetName = NewColumn[widget, string]("widgets", "name")
	widgetQty  = NewColumn[widget, int64]("widgets", "quantity")
	widgetBio  = NewNullableColumn[widget, string]("widgets", "bio")
)

// newWidgetsDB opens an in-memory sqlite database seeded with three
// widgets, one of which has a NULL bio -- proving the Option[string]/
// NullableColumn path end to end. Each call opens a fresh adapter, so
// tests never share mutable database state.
func newWidgetsDB(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx := context.Background()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id text, name text, quantity integer, bio text)`); err != nil {
		t.Fatalf("create table: %v", err)
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
			t.Fatalf("insert: %v", err)
		}
	}

	return ctx, conn
}

// openExampleDB opens the same seed data as newWidgetsDB, without a
// *testing.T, for use by Example functions (which get no *testing.T).
func openExampleDB(ctx context.Context) (db.DB, error) {
	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		return nil, err
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id text, name text, quantity integer, bio text)`); err != nil {
		return nil, err
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
			return nil, err
		}
	}

	return conn, nil
}

// fakeDB is a db.DB whose Dialect() name dialect.For does not
// recognize, exercising its error path without a panic or a silent SQLite
// fallback. Its other methods are never expected to be called: resolving
// the dialect fails before any of them would be reached.
type fakeDB struct{}

func (fakeDB) Query(context.Context, string, ...any) (db.Rows, error) {
	return &stubRows{}, nil
}
func (fakeDB) Exec(context.Context, string, ...any) (int64, error) { return 0, nil }
func (fakeDB) Ping(context.Context) error                          { return nil }
func (fakeDB) Close(context.Context) error                         { return nil }
func (fakeDB) Dialect() string                                     { return "unsupported-dialect" }

// loggedQuery is one captured (query, args) pair.
type loggedQuery struct {
	query string
	args  []any
}

// collectLogs installs a logger recording into *out and returns a restore
// func. Tests must restore (or set nil) so later tests start clean.
func collectLogs(out *[]loggedQuery) func() {
	return SetQueryLogger(func(query string, args []any) {
		*out = append(*out, loggedQuery{query: query, args: args})
	})
}

// newBigWidgetsDB opens an in-memory sqlite database seeded with n widgets
// (id, name, quantity, NULL bio), inserted in batches to keep seeding fast
// enough for CI. It backs the streaming peak-memory growth test
// (TestQueryStreamPeakMemoryFlatVersusAll).
func newBigWidgetsDB(t ormTestCleaner, n int) (context.Context, db.DB) {
	t.Helper()

	ctx := context.Background()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	if _, err := conn.Exec(ctx, `CREATE TABLE widgets (id text, name text, quantity integer, bio text)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	const batch = 500

	args := make([]any, 0, batch*3)
	var b strings.Builder

	for i := 0; i < n; i += batch {
		b.Reset()
		args = args[:0]

		b.WriteString("INSERT INTO widgets (id, name, quantity, bio) VALUES ")

		limit := min(i+batch, n)

		for j := i; j < limit; j++ {
			if j > i {
				b.WriteString(", ")
			}

			b.WriteString("(?, ?, ?, NULL)")

			args = append(args, fmt.Sprintf("w%06d", j), fmt.Sprintf("widget %d", j), int64(j))
		}

		if _, err := conn.Exec(ctx, b.String(), args...); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	return ctx, conn
}
