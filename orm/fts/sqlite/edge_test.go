package sqlite

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	orm "github.com/zenta-dev/zever/orm"
)

type captureExec struct {
	lastQuery string
	lastArgs  []any
}

func (c *captureExec) Query(_ context.Context, query string, args ...any) (db.Rows, error) {
	c.lastQuery = query
	c.lastArgs = args

	return emptyRows{}, nil
}

func (*captureExec) Exec(context.Context, string, ...any) (int64, error) { return 0, nil }
func (*captureExec) Ping(context.Context) error                          { return nil }
func (*captureExec) Close(context.Context) error                         { return nil }
func (*captureExec) Dialect() string                                     { return "sqlite" }

type emptyRows struct{}

func (emptyRows) Next() bool                 { return false }
func (emptyRows) Scan(...any) error          { return nil }
func (emptyRows) Close() error               { return nil }
func (emptyRows) Err() error                 { return nil }
func (emptyRows) Columns() ([]string, error) { return nil, nil }

func argsEqual(a, b []any) bool {
	return reflect.DeepEqual(a, b)
}

func TestSQLiteFTSMatchEmptyText(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(Match(docBody, "")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "title", "body" FROM "docs" WHERE "body" MATCH ?`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !argsEqual(capture.lastArgs, []any{""}) {
		t.Fatalf("args = %#v, want [\"\"]", capture.lastArgs)
	}
}

func TestSQLiteFTSMatchBooleanSyntaxStaysBound(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	// FTS5 query syntax operators must reach the engine as data, never inlined.
	text := `"memory safety" AND go OR rust NOT java NEAR(go, rust) ^go go*`
	if _, err := orm.From(docs).Where(Match(docBody, text)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if strings.Contains(capture.lastQuery, text) {
		t.Fatalf("FTS5 query text inlined into SQL: %q", capture.lastQuery)
	}

	if !argsEqual(capture.lastArgs, []any{text}) {
		t.Fatalf("args = %#v, want [%q]", capture.lastArgs, text)
	}
}

func TestSQLiteFTSMatchSpecialCharsStayBound(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	text := `sql & "acid" | !go 'quoted' \ % _`
	if _, err := orm.From(docs).Where(Match(docBody, text)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if strings.Contains(capture.lastQuery, text) {
		t.Fatalf("query text inlined into SQL: %q", capture.lastQuery)
	}

	if !argsEqual(capture.lastArgs, []any{text}) {
		t.Fatalf("args = %#v, want [%q]", capture.lastArgs, text)
	}
}

func TestSQLiteFTSMatchUnicodeText(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	text := "café ☕ 日本語"
	if _, err := orm.From(docs).Where(Match(docBody, text)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if !argsEqual(capture.lastArgs, []any{text}) {
		t.Fatalf("args = %#v, want [%q]", capture.lastArgs, text)
	}
}

func TestSQLiteFTSMatchOtherColumn(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(Match(docTitle, "go")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "title", "body" FROM "docs" WHERE "title" MATCH ?`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !argsEqual(capture.lastArgs, []any{"go"}) {
		t.Fatalf("args = %#v, want [go]", capture.lastArgs)
	}
}

func TestSQLiteFTSMatchComposesWithOr(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).
		Where(orm.Or(Match(docBody, "go"), Match(docBody, "rust"))).
		All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "title", "body" FROM "docs" WHERE ("body" MATCH ? OR "body" MATCH ?)`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !argsEqual(capture.lastArgs, []any{"go", "rust"}) {
		t.Fatalf("args = %#v, want [go rust]", capture.lastArgs)
	}
}

func TestSQLiteFTSRankRendersHiddenColumn(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	_, err := orm.From(docs).
		Where(Match(docBody, "go")).
		OrderBy(Rank(docBody)).
		All(ctx, capture)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "title", "body" FROM "docs" WHERE "body" MATCH ? ORDER BY "rank" DESC`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !argsEqual(capture.lastArgs, []any{"go"}) {
		t.Fatalf("args = %#v, want [go]", capture.lastArgs)
	}
}

func TestSQLiteFTSMatchLongText(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	text := strings.Repeat("word ", 500)
	if _, err := orm.From(docs).Where(Match(docBody, text)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if !argsEqual(capture.lastArgs, []any{text}) {
		t.Fatalf("long text not bound verbatim: got %d args", len(capture.lastArgs))
	}
}
