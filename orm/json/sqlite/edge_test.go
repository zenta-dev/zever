package sqlite

import (
	"context"
	"math"
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

func TestSQLiteJSONExtractTextEmptyKey(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(ExtractText(docData, "").Eq("x")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	// Empty key is not a "simple" key, so it renders double-quoted.
	want := `SELECT "id", "data" FROM "docs" WHERE json_extract("data", '$.""') = ?`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestSQLiteJSONExtractIndexNegative(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(ExtractIndexText(docData, -1).Eq("x")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE json_extract("data", '$[-1]') = ?`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestSQLiteJSONExtractIndexMaxInt64(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(ExtractIndexText(docData, math.MaxInt64).Eq("x")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE json_extract("data", '$[9223372036854775807]') = ?`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestSQLiteJSONKeyExistsEmptyKey(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(KeyExists(docData, "")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE json_extract("data", '$.""') IS NOT NULL`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestSQLiteJSONKeySpecialCharsStayBound(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	// The key is part of the json path literal, escaped by the renderer;
	// the comparison value stays a bound argument.
	key := "weird.key"
	if _, err := orm.From(docs).Where(KeyExists(docData, key)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if !strings.Contains(capture.lastQuery, `$."weird.key"`) {
		t.Fatalf("key not escaped into path literal: %q", capture.lastQuery)
	}
}

func TestSQLiteJSONArrayContainsEmptyElement(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(ArrayContains(docData, "")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE EXISTS (SELECT 1 FROM json_each("data") WHERE value = ?)`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if len(capture.lastArgs) != 1 || capture.lastArgs[0] != "" {
		t.Fatalf("args = %#v, want [\"\"]", capture.lastArgs)
	}
}

func TestSQLiteJSONTypeNoPath(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(Type(docData).Eq("object")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE json_type("data") = ?`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestSQLiteJSONChainedExtractEdge(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	pred := JSONPath(docData).Extract("tags").ExtractIndexText(0).Eq("go")
	if _, err := orm.From(docs).Where(pred).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE json_extract("data", '$.tags[0]') = ?`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestSQLiteJSONExtractIsNull(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(ExtractText(docData, "name").IsNull()).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE json_extract("data", '$.name') IS NULL`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestSQLiteJSONInEmptyList(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(ExtractText(docData, "name").In()).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	// Empty IN list is the constant false, rendered without args.
	want := `SELECT "id", "data" FROM "docs" WHERE 1 = 0`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if len(capture.lastArgs) != 0 {
		t.Fatalf("args = %#v, want none", capture.lastArgs)
	}
}
