package render

import (
	"errors"
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// jsonNode builds a JSON predicate node over the given operator/op/value,
// with the same shape the JSON predicate builder produces.
func jsonNode(op JSONOp, steps []JSONStep, cmp Op, value any) Node {
	return Node{Kind: KindJSON, Table: "docs", Column: "data", Op: cmp, Value: value, JSON: &JSONExpr{Op: op, Steps: steps}}
}

// jsonPathNode builds a JSON predicate node whose operand is a bound
// jsonpath expression (the Path field), mirroring the jsonpath builders.
func jsonPathNode(op JSONOp, steps []JSONStep, jsonpath string) Node {
	return Node{Kind: KindJSON, Table: "docs", Column: "data", JSON: &JSONExpr{Op: op, Steps: steps, Path: jsonpath}}
}

// jsonPathCmpNode is jsonPathNode plus a comparison Op/Value, for
// jsonb_path_query_first nodes.
func jsonPathCmpNode(op JSONOp, jsonpath string, cmp Op, value any) Node {
	return Node{Kind: KindJSON, Table: "docs", Column: "data", Op: cmp, Value: value, JSON: &JSONExpr{Op: op, Path: jsonpath}}
}

func TestRenderJSONExtract(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		node Node
		dial string
		want string
		args []any
	}{
		{
			"extract text single key / sqlite",
			jsonNode(JSONExtractText, []JSONStep{{Key: "name"}}, OpEq, "alice"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE json_extract("data", '$.name') = ?`,
			[]any{"alice"},
		},
		{
			"extract text single key / postgres",
			jsonNode(JSONExtractText, []JSONStep{{Key: "name"}}, OpEq, "alice"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data"->>'name' = $1`,
			[]any{"alice"},
		},
		{
			"extract text chained / postgres",
			jsonNode(JSONExtractText, []JSONStep{{Key: "author"}, {Key: "name"}}, OpEq, "ada"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data"->'author'->>'name' = $1`,
			[]any{"ada"},
		},
		{
			"extract text chained / sqlite",
			jsonNode(JSONExtractText, []JSONStep{{Key: "author"}, {Key: "name"}}, OpEq, "ada"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE json_extract("data", '$.author.name') = ?`,
			[]any{"ada"},
		},
		{
			"extract index text / postgres",
			jsonNode(JSONExtractText, []JSONStep{{Index: 0, IsIndex: true}}, OpEq, "go"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data"->>0 = $1`,
			[]any{"go"},
		},
		{
			"extract index / sqlite",
			jsonNode(JSONExtract, []JSONStep{{Index: 2, IsIndex: true}}, OpGt, "10"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE json_extract("data", '$[2]') > ?`,
			[]any{"10"},
		},
		{
			"path extract / postgres",
			jsonNode(JSONPathExtract, []JSONStep{{Key: "a"}, {Key: "b"}}, OpEq, `{"x":1}`),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data"#>'{"a","b"}' = $1::jsonb`,
			[]any{`{"x":1}`},
		},
		{
			"path extract text / postgres",
			jsonNode(JSONPathExtractText, []JSONStep{{Key: "a"}, {Key: "b"}}, OpEq, "v"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data"#>>'{"a","b"}' = $1`,
			[]any{"v"},
		},
		{
			"is null / sqlite",
			jsonNode(JSONExtractText, []JSONStep{{Key: "meta"}}, OpIsNull, nil),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE json_extract("data", '$.meta') IS NULL`,
			nil,
		},
		{
			"is not null / postgres",
			jsonNode(JSONExtractText, []JSONStep{{Key: "meta"}}, OpIsNotNull, nil),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data"->>'meta' IS NOT NULL`,
			nil,
		},
		{
			"in / sqlite",
			jsonNode(JSONExtractText, []JSONStep{{Key: "status"}}, OpIn, []any{"a", "b"}),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE json_extract("data", '$.status') IN (?, ?)`,
			[]any{"a", "b"},
		},
		{
			"special key quoted / sqlite",
			jsonNode(JSONExtractText, []JSONStep{{Key: "my key"}}, OpEq, "x"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE json_extract("data", '$."my key"') = ?`,
			[]any{"x"},
		},
		{
			"key with quote / postgres",
			jsonNode(JSONExtractText, []JSONStep{{Key: "o'brien"}}, OpEq, "x"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data"->>'o''brien' = $1`,
			[]any{"x"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q, args, err := Select(dialectFor(tc.dial), "docs", []string{"id"}, tc.node, nil, 0, 0)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if q != tc.want {
				t.Fatalf("query = %q, want %q", q, tc.want)
			}

			if !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("args = %#v, want %#v", args, tc.args)
			}
		})
	}
}

func TestRenderJSONOperators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		node Node
		dial string
		want string
		args []any
	}{
		{
			"contains / postgres",
			jsonNode(JSONContains, nil, OpEq, `{"role":"admin"}`),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data" @> $1::jsonb`,
			[]any{`{"role":"admin"}`},
		},
		{
			"contains with path / postgres",
			jsonNode(JSONContains, []JSONStep{{Key: "meta"}}, OpEq, `{"active":true}`),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data"->'meta' @> $1::jsonb`,
			[]any{`{"active":true}`},
		},
		{
			"key exists / postgres",
			jsonNode(JSONKeyExists, nil, OpEq, "meta"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data" ? $1`,
			[]any{"meta"},
		},
		{
			"key exists / sqlite",
			jsonNode(JSONKeyExists, nil, OpEq, "meta"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE json_extract("data", '$.meta') IS NOT NULL`,
			nil,
		},
		{
			"key exists chained / sqlite",
			jsonNode(JSONKeyExists, []JSONStep{{Key: "meta"}}, OpEq, "draft"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE json_extract("data", '$.meta.draft') IS NOT NULL`,
			nil,
		},
		{
			"key exists chained / postgres",
			jsonNode(JSONKeyExists, []JSONStep{{Key: "meta"}}, OpEq, "draft"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE "data"->'meta' ? $1`,
			[]any{"draft"},
		},
		{
			"array contains / sqlite",
			jsonNode(JSONArrayContains, nil, OpEq, "go"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE EXISTS (SELECT 1 FROM json_each("data") WHERE value = ?)`,
			[]any{"go"},
		},
		{
			"array contains with path / sqlite",
			jsonNode(JSONArrayContains, []JSONStep{{Key: "tags"}}, OpEq, "go"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE EXISTS (SELECT 1 FROM json_each("data", '$.tags') WHERE value = ?)`,
			[]any{"go"},
		},
		{
			"type / postgres",
			jsonNode(JSONType, nil, OpEq, "object"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE jsonb_typeof("data") = $1`,
			[]any{"object"},
		},
		{
			"type / sqlite",
			jsonNode(JSONType, []JSONStep{{Key: "a"}}, OpEq, "array"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE json_type("data", '$.a') = ?`,
			[]any{"array"},
		},
		{
			"type with path / postgres",
			jsonNode(JSONType, []JSONStep{{Key: "tags"}}, OpEq, "array"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE jsonb_typeof("data"->'tags') = $1`,
			[]any{"array"},
		},
		{
			"length / sqlite",
			jsonNode(JSONLength, nil, OpEq, int64(2)),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE JSON_LENGTH("data") = ?`,
			[]any{int64(2)},
		},
		{
			"length with path / sqlite",
			jsonNode(JSONLength, []JSONStep{{Key: "tags"}}, OpGt, int64(1)),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE JSON_LENGTH(JSON_EXTRACT("data", '$.tags')) > ?`,
			[]any{int64(1)},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q, args, err := Select(dialectFor(tc.dial), "docs", []string{"id"}, tc.node, nil, 0, 0)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if q != tc.want {
				t.Fatalf("query = %q, want %q", q, tc.want)
			}

			if !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("args = %#v, want %#v", args, tc.args)
			}
		})
	}
}

func TestRenderJSONQualifiedInJoin(t *testing.T) {
	t.Parallel()
	// A JSON predicate used as a join's left-side WHERE must be qualified
	// to its own table, exactly like a plain column predicate.
	where := qualifyNode(jsonNode(JSONExtractText, []JSONStep{{Key: "name"}}, OpEq, "alice"), "")

	q, args, err := Select(postgres.New(), "docs", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "docs" WHERE "docs"."data"->>'name' = $1`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"alice"}) {
		t.Fatalf("args = %#v, want [alice]", args)
	}
}

// TestRenderJSONPathOperators pins the Postgres-only jsonpath surface:
// `@?`, `@@`, the jsonb_path_exists/jsonb_path_match function forms,
// jsonb_path_query_first as a comparable value, and the `?|`/`?&`
// multi-key operators. Each jsonpath/key is a bound argument, and the
// placeholder numbering stays clause-ordered (path before comparison).
func TestRenderJSONPathOperators(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		node Node
		want string
		args []any
	}{
		{
			"path exists / postgres",
			jsonPathNode(JSONPathExists, nil, `$.a[*] ? (@ > 2)`),
			`SELECT "id" FROM "docs" WHERE "data" @? $1`,
			[]any{`$.a[*] ? (@ > 2)`},
		},
		{
			"path exists nested / postgres",
			jsonPathNode(JSONPathExists, []JSONStep{{Key: "meta"}}, `$.active`),
			`SELECT "id" FROM "docs" WHERE "data"->'meta' @? $1`,
			[]any{`$.active`},
		},
		{
			"path match / postgres",
			jsonPathNode(JSONPathMatch, nil, `$.a[*] > 2`),
			`SELECT "id" FROM "docs" WHERE "data" @@ $1`,
			[]any{`$.a[*] > 2`},
		},
		{
			"path exists func / postgres",
			jsonPathNode(JSONPathExistsFunc, nil, `$.a`),
			`SELECT "id" FROM "docs" WHERE jsonb_path_exists("data", $1)`,
			[]any{`$.a`},
		},
		{
			"path match func / postgres",
			jsonPathNode(JSONPathMatchFunc, nil, `$.a > 2`),
			`SELECT "id" FROM "docs" WHERE jsonb_path_match("data", $1)`,
			[]any{`$.a > 2`},
		},
		{
			"query first eq / postgres",
			jsonPathCmpNode(JSONPathQueryFirst, `$[0]`, OpEq, `{"x":1}`),
			`SELECT "id" FROM "docs" WHERE jsonb_path_query_first("data", $1) = $2::jsonb`,
			[]any{`$[0]`, `{"x":1}`},
		},
		{
			"query first is null / postgres",
			jsonPathCmpNode(JSONPathQueryFirst, `$[0]`, OpIsNull, nil),
			`SELECT "id" FROM "docs" WHERE jsonb_path_query_first("data", $1) IS NULL`,
			[]any{`$[0]`},
		},
		{
			"key exists any / postgres",
			jsonNode(JSONKeyExistsAny, nil, OpEq, []string{"a", "b"}),
			`SELECT "id" FROM "docs" WHERE "data" ?| ARRAY[$1, $2]`,
			[]any{"a", "b"},
		},
		{
			"key exists all / postgres",
			jsonNode(JSONKeyExistsAll, nil, OpEq, []string{"a", "b"}),
			`SELECT "id" FROM "docs" WHERE "data" ?& ARRAY[$1, $2]`,
			[]any{"a", "b"},
		},
		{
			"key exists any empty is false",
			jsonNode(JSONKeyExistsAny, nil, OpEq, []string{}),
			`SELECT "id" FROM "docs" WHERE 1 = 0`,
			nil,
		},
		{
			"key exists all empty is true",
			jsonNode(JSONKeyExistsAll, nil, OpEq, []string{}),
			`SELECT "id" FROM "docs" WHERE 1 = 1`,
			nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q, args, err := Select(postgres.New(), "docs", []string{"id"}, tc.node, nil, 0, 0)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if q != tc.want {
				t.Fatalf("query = %q, want %q", q, tc.want)
			}

			if !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("args = %#v, want %#v", args, tc.args)
			}
		})
	}
}

// TestRenderJSONPathUnsupportedDialect asserts the Postgres-only jsonpath
// operators fail closed with a typed dialect.ErrUnsupportedByDialect on
// sqlite instead of rendering invalid SQL or silently dropping the
// predicate.
func TestRenderJSONPathUnsupportedDialect(t *testing.T) {
	t.Parallel()
	for _, op := range []JSONOp{
		JSONPathExists, JSONPathMatch, JSONPathExistsFunc,
		JSONPathMatchFunc, JSONPathQueryFirst, JSONKeyExistsAny, JSONKeyExistsAll,
	} {
		node := jsonNode(op, nil, OpEq, "$.a")

		_, _, err := Select(sqlite.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
			t.Fatalf("op %d: err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", op, err)
		}
	}
}

func dialectFor(name string) dialect.Dialect {
	if name == "postgres" {
		return postgres.New()
	}

	return sqlite.New()
}

func TestRenderJSONOperatorErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("unknown operator rejected", func(t *testing.T) {
		t.Parallel()

		node := jsonNode(JSONOp(99), nil, OpEq, "x")

		_, _, err := Select(postgres.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want an unsupported-operator error")
		}
	})

	t.Run("empty in is always false", func(t *testing.T) {
		t.Parallel()

		node := jsonNode(JSONExtractText, []JSONStep{{Key: "status"}}, OpIn, []any{})

		q, args, err := Select(sqlite.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if q != `SELECT "id" FROM "docs" WHERE 1 = 0` {
			t.Fatalf("query = %q, want 1 = 0", q)
		}

		if args != nil {
			t.Fatalf("args = %#v, want nil", args)
		}
	})

	t.Run("non-comparison operator renders no clause", func(t *testing.T) {
		t.Parallel()

		node := jsonNode(JSONExtractText, []JSONStep{{Key: "status"}}, OpEqAny, "a")

		q, _, err := Select(sqlite.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `SELECT "id" FROM "docs"`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("like comparison renders no clause", func(t *testing.T) {
		t.Parallel()

		node := jsonNode(JSONExtractText, []JSONStep{{Key: "status"}}, OpLike, "a%")

		q, _, err := Select(sqlite.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `SELECT "id" FROM "docs"`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("unknown comparison renders no clause", func(t *testing.T) {
		t.Parallel()

		node := jsonNode(JSONExtractText, []JSONStep{{Key: "status"}}, Op(99), "a")

		q, _, err := Select(sqlite.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `SELECT "id" FROM "docs"`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

func TestRenderJSONPathKeyVarieties(t *testing.T) {
	t.Parallel()

	t.Run("index step in path extract array", func(t *testing.T) {
		t.Parallel()

		node := jsonNode(JSONPathExtract, []JSONStep{{Key: "a"}, {Index: 0, IsIndex: true}}, OpEq, `{"x":1}`)

		q, _, err := Select(postgres.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id" FROM "docs" WHERE "data"#>'{"a",0}' = $1::jsonb`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("no steps extracts whole document", func(t *testing.T) {
		t.Parallel()

		node := jsonNode(JSONExtractText, nil, OpEq, "alice")

		q, _, err := Select(sqlite.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id" FROM "docs" WHERE json_extract("data") = ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("mixed-char key needs no quoting", func(t *testing.T) {
		t.Parallel()

		node := jsonNode(JSONExtractText, []JSONStep{{Key: "_A9"}}, OpEq, "x")

		q, _, err := Select(sqlite.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id" FROM "docs" WHERE json_extract("data", '$._A9') = ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("empty key is quoted", func(t *testing.T) {
		t.Parallel()

		node := jsonNode(JSONExtractText, []JSONStep{{Key: ""}}, OpEq, "x")

		q, _, err := Select(sqlite.New(), "docs", []string{"id"}, node, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id" FROM "docs" WHERE json_extract("data", '$.""') = ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("unknown jsonpath op name falls back", func(t *testing.T) {
		t.Parallel()

		if got := jsonPathOpName(JSONOp(99)); got != "JSON" {
			t.Fatalf("jsonPathOpName(99) = %q, want JSON", got)
		}
	})

	t.Run("unknown extraction op renders bare column", func(t *testing.T) {
		t.Parallel()

		n := Node{Table: "docs", Column: "data", JSON: &JSONExpr{Op: JSONOp(99)}}

		if got := jsonExprText(postgres.New(), n); got != `"data"` {
			t.Fatalf("jsonExprText = %q, want the bare column", got)
		}
	})
}

func TestRenderJSONNilPayload(t *testing.T) {
	t.Parallel()

	q, _, err := Select(postgres.New(), "docs", []string{"id"}, Node{Kind: KindJSON}, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	if want := `SELECT "id" FROM "docs"`; q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}
