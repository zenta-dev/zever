package postgres

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	orm "github.com/zenta-dev/zever/orm"
)

// doc is a fixture entity with a jsonb column, mirroring the shape
// schema codegen generates for a `data: json` field.
type doc struct {
	ID   string
	Data string
}

func (d *doc) Scan(row orm.Row) error {
	return row.Scan(&d.ID, &d.Data)
}

var (
	docs    = orm.NewTable[doc]("docs", []string{"id", "data"})
	docData = orm.NewColumn[doc, string]("docs", "data")
)

// captureExec is a db.DB that records the last query/args it was asked to
// run and returns an empty result set -- enough to assert the exact
// Postgres SQL the JSON packages render, without needing a live Postgres.
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
func (*captureExec) Dialect() string                                     { return "postgres" }

type emptyRows struct{}

func (emptyRows) Next() bool                 { return false }
func (emptyRows) Scan(...any) error          { return nil }
func (emptyRows) Close() error               { return nil }
func (emptyRows) Err() error                 { return nil }
func (emptyRows) Columns() ([]string, error) { return nil, nil }

func TestPostgresJSONRendering(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name     string
		pred     func() orm.Predicate[doc]
		wantSQL  string
		wantArgs []any
	}{
		{
			"extract text single key",
			func() orm.Predicate[doc] { return ExtractText(docData, "name").Eq("alice") },
			`SELECT "id", "data" FROM "docs" WHERE "data"->>'name' = $1`,
			[]any{"alice"},
		},
		{
			"extract jsonb eq",
			func() orm.Predicate[doc] { return JSONPath(docData).Extract("meta").Eq(`{"active":true}`) },
			`SELECT "id", "data" FROM "docs" WHERE "data"->'meta' = $1::jsonb`,
			[]any{`{"active":true}`},
		},
		{
			"chained extract text",
			func() orm.Predicate[doc] { return JSONPath(docData).Extract("author").ExtractText("name").Eq("ada") },
			`SELECT "id", "data" FROM "docs" WHERE "data"->'author'->>'name' = $1`,
			[]any{"ada"},
		},
		{
			"extract index text",
			func() orm.Predicate[doc] { return ExtractIndexText(docData, 0).Eq("go") },
			`SELECT "id", "data" FROM "docs" WHERE "data"->>0 = $1`,
			[]any{"go"},
		},
		{
			"contains",
			func() orm.Predicate[doc] { return Contains(docData, `{"role":"admin"}`) },
			`SELECT "id", "data" FROM "docs" WHERE "data" @> $1::jsonb`,
			[]any{`{"role":"admin"}`},
		},
		{
			"key exists",
			func() orm.Predicate[doc] { return KeyExists(docData, "meta") },
			`SELECT "id", "data" FROM "docs" WHERE "data" ? $1`,
			[]any{"meta"},
		},
		{
			"key exists chained",
			func() orm.Predicate[doc] { return JSONPath(docData).Extract("meta").KeyExists("active") },
			`SELECT "id", "data" FROM "docs" WHERE "data"->'meta' ? $1`,
			[]any{"active"},
		},
		{
			"path extract",
			func() orm.Predicate[doc] { return PathExtract(docData, "a", "b").Eq(`{"x":1}`) },
			`SELECT "id", "data" FROM "docs" WHERE "data"#>'{"a","b"}' = $1::jsonb`,
			[]any{`{"x":1}`},
		},
		{
			"path extract text",
			func() orm.Predicate[doc] { return PathExtractText(docData, "a", "b").Eq("v") },
			`SELECT "id", "data" FROM "docs" WHERE "data"#>>'{"a","b"}' = $1`,
			[]any{"v"},
		},
		{
			"type",
			func() orm.Predicate[doc] { return Type(docData).Eq("object") },
			`SELECT "id", "data" FROM "docs" WHERE jsonb_typeof("data") = $1`,
			[]any{"object"},
		},
		{
			"is null",
			func() orm.Predicate[doc] { return JSONPath(docData).Extract("meta").IsNull() },
			`SELECT "id", "data" FROM "docs" WHERE "data"->'meta' IS NULL`,
			nil,
		},
		{
			"in",
			func() orm.Predicate[doc] { return ExtractText(docData, "role").In("admin", "user") },
			`SELECT "id", "data" FROM "docs" WHERE "data"->>'role' IN ($1, $2)`,
			[]any{"admin", "user"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capture := &captureExec{}

			if _, err := orm.From(docs).Where(tc.pred()).All(ctx, capture); err != nil {
				t.Fatalf("All: %v", err)
			}

			if capture.lastQuery != tc.wantSQL {
				t.Fatalf("SQL = %q, want %q", capture.lastQuery, tc.wantSQL)
			}

			if !equalArgs(capture.lastArgs, tc.wantArgs) {
				t.Fatalf("args = %#v, want %#v", capture.lastArgs, tc.wantArgs)
			}
		})
	}
}

// TestPostgresJSONCompilesWithAnd proves JSON predicates compose through
// orm.And like any other predicate.
func TestPostgresJSONCompilesWithAnd(t *testing.T) {
	ctx := t.Context()

	capture := &captureExec{}

	_, err := orm.From(docs).
		Where(orm.And(
			ExtractText(docData, "role").Eq("admin"),
			KeyExists(docData, "meta"),
		)).
		All(ctx, capture)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE ("data"->>'role' = $1 AND "data" ? $2)`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !equalArgs(capture.lastArgs, []any{"admin", "meta"}) {
		t.Fatalf("args = %#v, want [admin meta]", capture.lastArgs)
	}
}

func TestPostgresJSONPathRendering(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name     string
		pred     func() orm.Predicate[doc]
		wantSQL  string
		wantArgs []any
	}{
		{
			"path exists",
			func() orm.Predicate[doc] { return PathExists(docData, `$.tags[*] ? (@ == "go")`) },
			`SELECT "id", "data" FROM "docs" WHERE "data" @? $1`,
			[]any{`$.tags[*] ? (@ == "go")`},
		},
		{
			"path exists nested",
			func() orm.Predicate[doc] { return JSONPath(docData).Extract("meta").PathExists(`$.active`) },
			`SELECT "id", "data" FROM "docs" WHERE "data"->'meta' @? $1`,
			[]any{`$.active`},
		},
		{
			"path match",
			func() orm.Predicate[doc] { return PathMatch(docData, `$.meta.active == true`) },
			`SELECT "id", "data" FROM "docs" WHERE "data" @@ $1`,
			[]any{`$.meta.active == true`},
		},
		{
			"path exists func",
			func() orm.Predicate[doc] { return PathExistsFunc(docData, `$.tags`) },
			`SELECT "id", "data" FROM "docs" WHERE jsonb_path_exists("data", $1)`,
			[]any{`$.tags`},
		},
		{
			"path match func",
			func() orm.Predicate[doc] { return PathMatchFunc(docData, `$.meta.active == true`) },
			`SELECT "id", "data" FROM "docs" WHERE jsonb_path_match("data", $1)`,
			[]any{`$.meta.active == true`},
		},
		{
			"query first eq",
			func() orm.Predicate[doc] { return QueryFirst(docData, `$.tags[0]`).Eq(`"go"`) },
			`SELECT "id", "data" FROM "docs" WHERE jsonb_path_query_first("data", $1) = $2::jsonb`,
			[]any{`$.tags[0]`, `"go"`},
		},
		{
			"query first is null",
			func() orm.Predicate[doc] { return QueryFirst(docData, `$.missing`).IsNull() },
			`SELECT "id", "data" FROM "docs" WHERE jsonb_path_query_first("data", $1) IS NULL`,
			[]any{`$.missing`},
		},
		{
			"key exists any",
			func() orm.Predicate[doc] { return KeyExistsAny(docData, "name", "role") },
			`SELECT "id", "data" FROM "docs" WHERE "data" ?| ARRAY[$1, $2]`,
			[]any{"name", "role"},
		},
		{
			"key exists all",
			func() orm.Predicate[doc] { return KeyExistsAll(docData, "name", "role") },
			`SELECT "id", "data" FROM "docs" WHERE "data" ?& ARRAY[$1, $2]`,
			[]any{"name", "role"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capture := &captureExec{}

			if _, err := orm.From(docs).Where(tc.pred()).All(ctx, capture); err != nil {
				t.Fatalf("All: %v", err)
			}

			if capture.lastQuery != tc.wantSQL {
				t.Fatalf("SQL = %q, want %q", capture.lastQuery, tc.wantSQL)
			}

			if !equalArgs(capture.lastArgs, tc.wantArgs) {
				t.Fatalf("args = %#v, want %#v", capture.lastArgs, tc.wantArgs)
			}
		})
	}
}

// TestPostgresJSONPathPlaceholderOrdering proves a jsonpath predicate
// composes with a normal extraction predicate through orm.And with
// placeholder numbering staying clause-ordered.
func TestPostgresJSONPathPlaceholderOrdering(t *testing.T) {
	ctx := t.Context()

	capture := &captureExec{}

	_, err := orm.From(docs).
		Where(orm.And(
			PathExists(docData, `$.tags[*] ? (@ == "go")`),
			ExtractText(docData, "name").Eq("alice"),
		)).
		All(ctx, capture)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "data" FROM "docs" WHERE ("data" @? $1 AND "data"->>'name' = $2)`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !equalArgs(capture.lastArgs, []any{`$.tags[*] ? (@ == "go")`, "alice"}) {
		t.Fatalf("args = %#v, want [jsonpath alice]", capture.lastArgs)
	}
}

func equalArgs(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

// TestPostgresBuilderCoverage exercises every builder and comparison
// through Postgres rendering: each predicate must render without error
// against the postgres dialect, proving the full fluent surface composes.
func TestPostgresBuilderCoverage(t *testing.T) {
	ctx := t.Context()

	builders := []struct {
		name string
		pred orm.Predicate[doc]
	}{
		{"Extract", Extract(docData, "name").Eq(`"alice"`)},
		{"ExtractIndex", ExtractIndex(docData, 0).Eq(`"x"`)},
		{"PathExtract", JSONPath(docData).PathExtract("meta", "active").Eq(`{}`)},
		{"ExtractText", ExtractText(docData, "name").Eq("alice")},
		{"Path.Eq", JSONPath(docData).Eq(`{"a":1}`)},
		{"Path.Neq", JSONPath(docData).Neq(`{"a":1}`)},
		{"Path.IsNull", JSONPath(docData).IsNull()},
		{"Path.IsNotNull", JSONPath(docData).IsNotNull()},
		{"Contains", JSONPath(docData).Contains(`{"role":"admin"}`)},
		{"KeyExists", JSONPath(docData).KeyExists("name")},
		{"KeyExistsAny", JSONPath(docData).KeyExistsAny("a", "b")},
		{"KeyExistsAll", JSONPath(docData).KeyExistsAll("a", "b")},
		{"PathExists", JSONPath(docData).PathExists("$.a")},
		{"PathMatch", JSONPath(docData).PathMatch("$.a > 1")},
		{"PathExistsFunc", JSONPath(docData).PathExistsFunc("$.a")},
		{"PathMatchFunc", JSONPath(docData).PathMatchFunc("$.a > 1")},
		{"QueryFirst.Eq", QueryFirst(docData, "$.name").Eq(`"alice"`)},
		{"First.Neq", JSONPath(docData).QueryFirst("$.name").Neq(`"bob"`)},
		{"First.IsNull", JSONPath(docData).QueryFirst("$.name").IsNull()},
		{"First.IsNotNull", JSONPath(docData).QueryFirst("$.name").IsNotNull()},
		{"Text.Gt", ExtractText(docData, "name").Gt("a")},
		{"Text.Gte", ExtractText(docData, "name").Gte("alice")},
		{"Text.Lt", ExtractText(docData, "name").Lt("z")},
		{"Text.Lte", ExtractText(docData, "name").Lte("alice")},
		{"Text.In", ExtractText(docData, "name").In("alice", "bob")},
		{"Text.IsNull", ExtractText(docData, "name").IsNull()},
		{"Text.IsNotNull", ExtractText(docData, "name").IsNotNull()},
		{"Text.Neq", ExtractText(docData, "name").Neq("bob")},
	}

	for _, tc := range builders {
		t.Run(tc.name, func(t *testing.T) {
			conn := &captureExec{}

			if _, err := orm.From(docs).Where(tc.pred).All(ctx, conn); err != nil {
				t.Fatalf("All: %v", err)
			}

			if conn.lastQuery == "" {
				t.Fatal("no query captured")
			}
		})
	}

	p := JSONPath(docData)
	_ = p.Extract("a")
	_ = p.ExtractIndex(0)
	_ = p.ExtractText("a")
	_ = p.ExtractIndexText(0)
	_ = p.PathExtractText("meta")
	_ = ExtractIndexText(docData, 0)
	_ = PathExtractText(docData, "meta")
}
