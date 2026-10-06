package postgres

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	orm "github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
)

type captureSQLite struct {
	lastQuery string
	lastArgs  []any
}

func (c *captureSQLite) Query(_ context.Context, query string, args ...any) (db.Rows, error) {
	c.lastQuery = query
	c.lastArgs = args

	return emptyRows{}, nil
}

func (*captureSQLite) Exec(context.Context, string, ...any) (int64, error) { return 0, nil }
func (*captureSQLite) Ping(context.Context) error                          { return nil }
func (*captureSQLite) Close(context.Context) error                         { return nil }
func (*captureSQLite) Dialect() string                                     { return "sqlite" }

func TestPostgresJSONExtractTextEmptyKey(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(ExtractText(docData, "").Eq("x")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data"->>'' = $1`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestPostgresJSONExtractIndexNegative(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(ExtractIndexText(docData, -1).Eq("x")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data"->>-1 = $1`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestPostgresJSONExtractIndexMaxInt64(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(ExtractIndexText(docData, math.MaxInt64).Eq("x")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data"->>9223372036854775807 = $1`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestPostgresJSONKeyExistsAnyEmptyKeyList(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	// Vacuous "any of {}" is the constant false, rendered without args.
	if _, err := orm.From(docs).Where(KeyExistsAny(docData)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE 1 = 0`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if len(capture.lastArgs) != 0 {
		t.Fatalf("args = %#v, want none", capture.lastArgs)
	}
}

func TestPostgresJSONKeyExistsAllEmptyKeyList(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	// Vacuous "all of {}" is the constant true, rendered without args.
	if _, err := orm.From(docs).Where(KeyExistsAll(docData)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE 1 = 1`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if len(capture.lastArgs) != 0 {
		t.Fatalf("args = %#v, want none", capture.lastArgs)
	}
}

func TestPostgresJSONPathExtractNoKeys(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(JSONPath(docData).PathExtract().Eq(`{"a":1}`)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data"#>'{}' = $1::jsonb`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestPostgresJSONPathExtractTextNoKeys(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(JSONPath(docData).PathExtractText().Eq("x")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data"#>>'{}' = $1`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestPostgresJSONContainsEmptyDoc(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(Contains(docData, "")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data" @> $1::jsonb`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if len(capture.lastArgs) != 1 || capture.lastArgs[0] != "" {
		t.Fatalf("args = %#v, want [\"\"]", capture.lastArgs)
	}
}

func TestPostgresJSONPathExistsEmptyPath(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(PathExists(docData, "")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data" @? $1`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if len(capture.lastArgs) != 1 || capture.lastArgs[0] != "" {
		t.Fatalf("args = %#v, want [\"\"]", capture.lastArgs)
	}
}

func TestPostgresJSONPathMatchEmptyPath(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(PathMatch(docData, "")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data" @@ $1`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestPostgresJSONTypeEq(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(Type(docData).Eq("object")).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE jsonb_typeof("data") = $1`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestPostgresJSONQueryFirstEmptyPath(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	if _, err := orm.From(docs).Where(QueryFirst(docData, "").Eq(`{"a":1}`)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE jsonb_path_query_first("data", $1) = $2::jsonb`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if len(capture.lastArgs) != 2 || capture.lastArgs[0] != "" {
		t.Fatalf("args = %#v, want [\"\" doc]", capture.lastArgs)
	}
}

func TestPostgresJSONKeySpecialCharsStayBound(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	key := `weird"key\with'specials`
	if _, err := orm.From(docs).Where(KeyExists(docData, key)).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	if strings.Contains(capture.lastQuery, key) {
		t.Fatalf("key inlined into SQL: %q", capture.lastQuery)
	}

	if len(capture.lastArgs) != 1 || capture.lastArgs[0] != key {
		t.Fatalf("args = %#v, want [%q]", capture.lastArgs, key)
	}
}

func TestPostgresJSONChainedMixedSteps(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	pred := JSONPath(docData).Extract("a").ExtractIndex(2).ExtractText("b").Eq("x")
	if _, err := orm.From(docs).Where(pred).All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data"->'a'->2->>'b' = $1`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestPostgresJSONPathExtractKeyEscaping(t *testing.T) {
	ctx := t.Context()
	capture := &captureExec{}

	// Keys with quotes/backslashes are escaped inside the array literal,
	// never interpolated raw.
	if _, err := orm.From(docs).
		Where(JSONPath(docData).PathExtract(`a"b`, `c\d`).Eq(`{}`)).
		All(ctx, capture); err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE "data"#>'{"a\"b","c\\d"}' = $1::jsonb`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func TestPostgresJSONPathOpsUnsupportedOnSQLite(t *testing.T) {
	ctx := t.Context()
	capture := &captureSQLite{}

	_, err := orm.From(docs).Where(PathExists(docData, "$.a")).All(ctx, capture)
	if err == nil {
		t.Fatal("PathExists on sqlite dialect: expected error, got nil")
	}

	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want ErrUnsupportedByDialect", err)
	}
}

func TestPostgresJSONKeyExistsAnyUnsupportedOnSQLite(t *testing.T) {
	ctx := t.Context()
	capture := &captureSQLite{}

	_, err := orm.From(docs).Where(KeyExistsAny(docData, "a", "b")).All(ctx, capture)
	if err == nil {
		t.Fatal("KeyExistsAny on sqlite dialect: expected error, got nil")
	}

	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want ErrUnsupportedByDialect", err)
	}
}
