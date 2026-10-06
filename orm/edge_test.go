package orm

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
)

type edgeCaptureDB struct {
	dialect   string
	lastQuery string
	lastArgs  []any
}

func (c *edgeCaptureDB) Query(_ context.Context, query string, args ...any) (db.Rows, error) {
	c.lastQuery = query
	c.lastArgs = args

	return &stubRows{}, nil
}

func (c *edgeCaptureDB) Exec(context.Context, string, ...any) (int64, error) { return 0, nil }
func (c *edgeCaptureDB) Ping(context.Context) error                          { return nil }
func (c *edgeCaptureDB) Close(context.Context) error                         { return nil }
func (c *edgeCaptureDB) Dialect() string                                     { return c.dialect }

func TestEdgeUnsafeIdentEmpty(t *testing.T) {
	_, err := UnsafeIdent[widget, string]("", []string{"id"})
	if err == nil {
		t.Fatal("UnsafeIdent empty: want error, got nil")
	}

	if !errors.Is(err, ErrEmptyIdent) {
		t.Fatalf("err = %v, want ErrEmptyIdent", err)
	}
}

func TestEdgeUnsafeIdentNotInAllowlist(t *testing.T) {
	_, err := UnsafeIdent[widget, string]("name; DROP TABLE widgets", []string{"id", "name"})
	if err == nil {
		t.Fatal("UnsafeIdent outside allowlist: want error, got nil")
	}

	if !strings.Contains(err.Error(), "not in allowlist") {
		t.Fatalf("err = %v, want not-in-allowlist message", err)
	}
}

func TestEdgeUnsafeIdentAllowlistHit(t *testing.T) {
	col, err := UnsafeIdent[widget, string]("name", []string{"id", "name"})
	if err != nil {
		t.Fatalf("UnsafeIdent allowlist hit: %v", err)
	}

	if col.Name() != "name" {
		t.Fatalf("column name = %q, want %q", col.Name(), "name")
	}
}

func TestEdgeUnsafeRawNoMarkerBindsNothing(t *testing.T) {
	ctx := t.Context()
	capture := &edgeCaptureDB{dialect: "postgres"}

	if _, err := From(widgets).Where(UnsafeRaw[widget]("1 = 1")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(capture.lastArgs) != 0 {
		t.Fatalf("args = %#v, want none", capture.lastArgs)
	}
}

func TestEdgeUnsafeRawArgsStayBound(t *testing.T) {
	ctx := t.Context()
	capture := &edgeCaptureDB{dialect: "postgres"}

	// The fragment is a constant; the semicolon/quote payload travels as a
	// bound value, never as SQL structure.
	if _, err := From(widgets).
		Where(UnsafeRaw[widget]("name LIKE ? OR name = ?", "%go'; --", "x")).
		All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(capture.lastArgs) != 2 {
		t.Fatalf("args = %#v, want 2 bound values", capture.lastArgs)
	}

	if capture.lastArgs[0] != "%go'; --" {
		t.Fatalf("arg0 = %#v, want bound verbatim", capture.lastArgs[0])
	}
}

func TestEdgeUnsafeRawQuestionMarkersRenumbered(t *testing.T) {
	ctx := t.Context()
	capture := &edgeCaptureDB{dialect: "postgres"}

	if _, err := From(widgets).
		Where(UnsafeRaw[widget]("name = ? AND quantity > ?", "alpha", 3)).
		All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if !strings.Contains(capture.lastQuery, "$1") || !strings.Contains(capture.lastQuery, "$2") {
		t.Fatalf("? markers not renumbered: %q", capture.lastQuery)
	}
}

func TestEdgePreloadEmptyParentsSkipsChildQuery(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	// Parents match nothing: Preload must return (nil, nil) without ever
	// issuing the child query.
	parents := From(widgets).Where(widgetID.Eq("no-such-widget"))

	got, err := Preload(ctx, conn, parents, widgetID, From(widgets), func(*widget) string { return "" }, func(*widget) string { return "" })
	if err != nil {
		t.Fatalf("Preload empty parents: %v", err)
	}

	if got != nil {
		t.Fatalf("Preload empty parents = %v, want nil", got)
	}
}

func TestEdgeMergeNoWhensRejected(t *testing.T) {
	// A MERGE with no WHEN clause is a caller error: render rejects it
	// before any database round-trip.
	m := MergeInto(widgets).UsingSource(widgets).On(widgetID.Col(), widgetID.Col())

	_, err := m.Exec(t.Context(), &edgeCaptureDB{dialect: "postgres"})
	if err == nil {
		t.Fatal("Merge.Exec no whens: want error, got nil")
	}
}

func TestEdgeMergeUnsupportedDialect(t *testing.T) {
	m := MergeInto(widgets).
		UsingSource(widgets).
		On(widgetID.Col(), widgetID.Col()).
		WhenMatchedUpdate(MergeValue(widgetName.Col(), "delta"))

	_, err := m.Exec(t.Context(), &edgeCaptureDB{dialect: "sqlite"})
	if err == nil {
		t.Fatal("Merge.Exec sqlite: want error, got nil")
	}

	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want ErrUnsupportedByDialect", err)
	}
}

func TestEdgeSetQueryLoggerNilRestores(t *testing.T) {
	restore := SetQueryLogger(func(string, []any) {})
	restore()

	// After restore, logging is a no-op: a query must not panic or race.
	ctx, conn := newWidgetsDB(t)

	if _, err := From(widgets).All(ctx, conn); err != nil {
		t.Fatalf("All after logger restore: %v", err)
	}
}
