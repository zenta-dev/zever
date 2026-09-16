// Package postgres provides typed helpers over Postgres jsonb operators
// (`->`, `->>`, `@>`, `?`, `#>`, `#>>`), built as orm predicates that
// compose into orm.From(...).Where(...) exactly like Column methods do:
//
//	postgres.JSONPath(col).Contains(`{"role":"admin"}`) // col @> ?
//	postgres.ExtractText(col, "name").Eq("alice") // col->>'name' = ?
//	postgres.JSONPath(col).Extract("meta").KeyExists("draft") // col->'meta' ? ?
//
// Every helper returns a orm.Predicate[T] (or a typed path value whose
// comparison methods do), so the whole surface composes with the existing
// query builders -- no raw SQL at the call site. The abstract JSON operator
// the helpers build renders as Postgres jsonb operators when the query runs
// on Postgres, and as sqlite json1 functions when it runs on SQLite (see
// orm/render/json.go); the three orm/json/* packages (postgres, sqlite,
// mysql) are parallel surfaces over that same abstraction.
package postgres

import (
	orm "github.com/zenta-dev/zever/orm"
)

// Path is a jsonb-valued JSON path expression over entity T: a base JSON
// column plus an optional chain of extraction steps. It is the postgres
// rendering of `col->'a'->'b'` / `col#>'{a,b}'`.
type Path[T any] struct {
	table string
	col   string
	steps []orm.JSONStep
	op    orm.JSONOp
}

// JSONPath roots a jsonb path at a JSON column of entity T.
func JSONPath[T any, V any](c orm.Column[T, V]) Path[T] {
	return Path[T]{table: c.Table(), col: c.Name(), op: orm.JSONExtract}
}

// Extract appends an object-key extraction step (`col->'key'`, jsonb
// result).
func (p Path[T]) Extract(key string) Path[T] {
	p.steps = append(append([]orm.JSONStep(nil), p.steps...), orm.JSONKey(key))
	p.op = orm.JSONExtract

	return p
}

// ExtractIndex appends an array-index extraction step (`col->0`, jsonb
// result).
func (p Path[T]) ExtractIndex(i int64) Path[T] {
	p.steps = append(append([]orm.JSONStep(nil), p.steps...), orm.JSONIndex(i))
	p.op = orm.JSONExtract

	return p
}

// PathExtract replaces the path with a full `#>` path extract
// (`col#>'{a,b}'`, jsonb result). keys are the top-to-bottom path steps.
func (p Path[T]) PathExtract(keys ...string) Path[T] {
	p.steps = jsonKeys(keys)
	p.op = orm.JSONPathExtract

	return p
}

// ExtractText ends the path as a text extraction (`col->>'key'`).
func (p Path[T]) ExtractText(key string) Text[T] {
	steps := append(append([]orm.JSONStep(nil), p.steps...), orm.JSONKey(key))

	return Text[T]{table: p.table, col: p.col, steps: steps, op: orm.JSONExtractText}
}

// ExtractIndexText ends the path as a text extraction of an array element
// (`col->>0`).
func (p Path[T]) ExtractIndexText(i int64) Text[T] {
	steps := append(append([]orm.JSONStep(nil), p.steps...), orm.JSONIndex(i))

	return Text[T]{table: p.table, col: p.col, steps: steps, op: orm.JSONExtractText}
}

// PathExtractText ends the path as a `#>>` text path extract
// (`col#>>'{a,b}'`).
func (p Path[T]) PathExtractText(keys ...string) Text[T] {
	return Text[T]{table: p.table, col: p.col, steps: jsonKeys(keys), op: orm.JSONPathExtractText}
}

// Contains builds a `path @> doc` jsonb containment predicate.
func (p Path[T]) Contains(doc string) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONContains, Steps: p.steps}, orm.Eq, doc)
}

// KeyExists builds a `path ? key` key-existence predicate.
func (p Path[T]) KeyExists(key string) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONKeyExists, Steps: p.steps}, orm.Eq, key)
}

// KeyExistsAny builds a `path ?| ARRAY[keys...]` predicate: does the jsonb
// value have AT LEAST ONE of the named top-level keys? Each key is a
// separate bound argument.
func (p Path[T]) KeyExistsAny(keys ...string) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONKeyExistsAny, Steps: p.steps}, orm.Eq, keys)
}

// KeyExistsAll builds a `path ?& ARRAY[keys...]` predicate: does the jsonb
// value have ALL of the named top-level keys? Each key is a separate bound
// argument.
func (p Path[T]) KeyExistsAll(keys ...string) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONKeyExistsAll, Steps: p.steps}, orm.Eq, keys)
}

// PathExists builds a `path @? jsonpath` predicate: does the SQL/JSON path
// expression yield at least one item for the jsonb value? jsonpath is the
// Postgres jsonpath text (e.g. `$.a[*] ? (@ > 2)`), bound as an argument --
// never inlined.
func (p Path[T]) PathExists(jsonpath string) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONPathExists, Steps: p.steps, Path: jsonpath}, orm.Eq, nil)
}

// PathMatch builds a `path @@ jsonpath` predicate: does the SQL/JSON path
// predicate evaluate to true for the jsonb value? Unlike PathExists, the
// jsonpath must be a boolean predicate (e.g. `$.a[*] > 2`). Bound as an
// argument.
func (p Path[T]) PathMatch(jsonpath string) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONPathMatch, Steps: p.steps, Path: jsonpath}, orm.Eq, nil)
}

// PathExistsFunc builds a `jsonb_path_exists(path, jsonpath)` predicate --
// the function form of PathExists (useful when a jsonb timeout/silent
// variant, `jsonb_path_exists_tz`, is desired through UnsafeRaw). Bound as
// an argument.
func (p Path[T]) PathExistsFunc(jsonpath string) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONPathExistsFunc, Steps: p.steps, Path: jsonpath}, orm.Eq, nil)
}

// PathMatchFunc builds a `jsonb_path_match(path, jsonpath)` predicate -- the
// function form of PathMatch. Bound as an argument.
func (p Path[T]) PathMatchFunc(jsonpath string) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONPathMatchFunc, Steps: p.steps, Path: jsonpath}, orm.Eq, nil)
}

// QueryFirst builds a `jsonb_path_query_first(path, jsonpath)` value: the
// first jsonb item the jsonpath matches, comparable against a JSON document
// via the returned First's comparison methods. An unmatched path yields
// SQL NULL, so IsNull/IsNotNull work as expected.
func (p Path[T]) QueryFirst(jsonpath string) First[T] {
	return First[T]{table: p.table, col: p.col, steps: p.steps, path: jsonpath}
}

// First is a jsonb-valued jsonpath query over entity T: the result of
// `jsonb_path_query_first`, comparable against a JSON document string. It is
// returned by Path.QueryFirst and the package-level QueryFirst.
type First[T any] struct {
	table string
	col   string
	steps []orm.JSONStep
	path  string
}

// Eq builds a `jsonb_path_query_first(...) = doc::jsonb` predicate.
func (q First[T]) Eq(doc string) orm.Predicate[T] { return q.cmp(orm.Eq, doc) }

// Neq builds a `jsonb_path_query_first(...) <> doc::jsonb` predicate.
func (q First[T]) Neq(doc string) orm.Predicate[T] { return q.cmp(orm.Neq, doc) }

// IsNull builds a `jsonb_path_query_first(...) IS NULL` predicate (true when
// the path matches nothing).
func (q First[T]) IsNull() orm.Predicate[T] { return q.cmp(orm.IsNull, nil) }

// IsNotNull builds a `jsonb_path_query_first(...) IS NOT NULL` predicate.
func (q First[T]) IsNotNull() orm.Predicate[T] { return q.cmp(orm.IsNotNull, nil) }

func (q First[T]) cmp(op orm.Op, value any) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](q.table, q.col, orm.JSONExpr{Op: orm.JSONPathQueryFirst, Steps: q.steps, Path: q.path}, op, value)
}

// Eq builds a `path = doc::jsonb` predicate, comparing the extracted jsonb
// value against a JSON document string.
func (p Path[T]) Eq(doc string) orm.Predicate[T] { return p.cmp(orm.Eq, doc) }

// Neq builds a `path != doc::jsonb` predicate.
func (p Path[T]) Neq(doc string) orm.Predicate[T] { return p.cmp(orm.Neq, doc) }

// IsNull builds a `path IS NULL` predicate.
func (p Path[T]) IsNull() orm.Predicate[T] { return p.cmp(orm.IsNull, nil) }

// IsNotNull builds a `path IS NOT NULL` predicate.
func (p Path[T]) IsNotNull() orm.Predicate[T] { return p.cmp(orm.IsNotNull, nil) }

func (p Path[T]) cmp(op orm.Op, value any) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: p.op, Steps: p.steps}, op, value)
}

// Text is a text-valued JSON extraction over entity T: the result of `->>`,
// `#>>` or `jsonb_typeof`, comparable against Go strings.
type Text[T any] struct {
	table string
	col   string
	steps []orm.JSONStep
	op    orm.JSONOp
}

// Eq builds a `<extract> = v` predicate.
func (t Text[T]) Eq(v string) orm.Predicate[T] { return t.cmp(orm.Eq, v) }

// Neq builds a `<extract> != v` predicate.
func (t Text[T]) Neq(v string) orm.Predicate[T] { return t.cmp(orm.Neq, v) }

// Gt builds a `<extract> > v` predicate.
func (t Text[T]) Gt(v string) orm.Predicate[T] { return t.cmp(orm.Gt, v) }

// Gte builds a `<extract> >= v` predicate.
func (t Text[T]) Gte(v string) orm.Predicate[T] { return t.cmp(orm.Gte, v) }

// Lt builds a `<extract> < v` predicate.
func (t Text[T]) Lt(v string) orm.Predicate[T] { return t.cmp(orm.Lt, v) }

// Lte builds a `<extract> <= v` predicate.
func (t Text[T]) Lte(v string) orm.Predicate[T] { return t.cmp(orm.Lte, v) }

// In builds a `<extract> IN (vs...)` predicate.
func (t Text[T]) In(vs ...string) orm.Predicate[T] {
	vals := make([]any, len(vs))
	for i, v := range vs {
		vals[i] = v
	}

	return t.cmp(orm.In, vals)
}

// IsNull builds a `<extract> IS NULL` predicate.
func (t Text[T]) IsNull() orm.Predicate[T] { return t.cmp(orm.IsNull, nil) }

// IsNotNull builds a `<extract> IS NOT NULL` predicate.
func (t Text[T]) IsNotNull() orm.Predicate[T] { return t.cmp(orm.IsNotNull, nil) }

func (t Text[T]) cmp(op orm.Op, value any) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](t.table, t.col, orm.JSONExpr{Op: t.op, Steps: t.steps}, op, value)
}

// Extract builds a jsonb key extraction at a column (`col->'key'`).
func Extract[T any, V any](c orm.Column[T, V], key string) Path[T] {
	return JSONPath(c).Extract(key)
}

// ExtractIndex builds a jsonb array-index extraction (`col->0`).
func ExtractIndex[T any, V any](c orm.Column[T, V], i int64) Path[T] {
	return JSONPath(c).ExtractIndex(i)
}

// ExtractText builds a text key extraction (`col->>'key'`).
func ExtractText[T any, V any](c orm.Column[T, V], key string) Text[T] {
	return JSONPath(c).ExtractText(key)
}

// ExtractIndexText builds a text array-index extraction (`col->>0`).
func ExtractIndexText[T any, V any](c orm.Column[T, V], i int64) Text[T] {
	return JSONPath(c).ExtractIndexText(i)
}

// PathExtract builds a `#>` path extract at a column (`col#>'{a,b}'`).
func PathExtract[T any, V any](c orm.Column[T, V], keys ...string) Path[T] {
	return JSONPath(c).PathExtract(keys...)
}

// PathExtractText builds a `#>>` text path extract at a column
// (`col#>>'{a,b}'`).
func PathExtractText[T any, V any](c orm.Column[T, V], keys ...string) Text[T] {
	return JSONPath(c).PathExtractText(keys...)
}

// Contains builds a `col @> doc` jsonb containment predicate.
func Contains[T any, V any](c orm.Column[T, V], doc string) orm.Predicate[T] {
	return JSONPath(c).Contains(doc)
}

// KeyExists builds a `col ? key` key-existence predicate.
func KeyExists[T any, V any](c orm.Column[T, V], key string) orm.Predicate[T] {
	return JSONPath(c).KeyExists(key)
}

// KeyExistsAny builds a `col ?| ARRAY[keys...]` predicate: does the jsonb
// value have at least one of the named top-level keys?
func KeyExistsAny[T any, V any](c orm.Column[T, V], keys ...string) orm.Predicate[T] {
	return JSONPath(c).KeyExistsAny(keys...)
}

// KeyExistsAll builds a `col ?& ARRAY[keys...]` predicate: does the jsonb
// value have all of the named top-level keys?
func KeyExistsAll[T any, V any](c orm.Column[T, V], keys ...string) orm.Predicate[T] {
	return JSONPath(c).KeyExistsAll(keys...)
}

// PathExists builds a `col @? jsonpath` predicate (bound jsonpath text).
func PathExists[T any, V any](c orm.Column[T, V], jsonpath string) orm.Predicate[T] {
	return JSONPath(c).PathExists(jsonpath)
}

// PathMatch builds a `col @@ jsonpath` predicate (bound jsonpath text).
func PathMatch[T any, V any](c orm.Column[T, V], jsonpath string) orm.Predicate[T] {
	return JSONPath(c).PathMatch(jsonpath)
}

// PathExistsFunc builds a `jsonb_path_exists(col, jsonpath)` predicate.
func PathExistsFunc[T any, V any](c orm.Column[T, V], jsonpath string) orm.Predicate[T] {
	return JSONPath(c).PathExistsFunc(jsonpath)
}

// PathMatchFunc builds a `jsonb_path_match(col, jsonpath)` predicate.
func PathMatchFunc[T any, V any](c orm.Column[T, V], jsonpath string) orm.Predicate[T] {
	return JSONPath(c).PathMatchFunc(jsonpath)
}

// QueryFirst builds a `jsonb_path_query_first(col, jsonpath)` value,
// comparable against a JSON document.
func QueryFirst[T any, V any](c orm.Column[T, V], jsonpath string) First[T] {
	return JSONPath(c).QueryFirst(jsonpath)
}

// Type builds a `jsonb_typeof(col)` expression, comparable against the
// jsonb type name ("object", "array", "string", "number", "boolean",
// "null").
func Type[T any, V any](c orm.Column[T, V]) Text[T] {
	return Text[T]{table: c.Table(), col: c.Name(), op: orm.JSONType}
}

func jsonKeys(keys []string) []orm.JSONStep {
	steps := make([]orm.JSONStep, len(keys))
	for i, k := range keys {
		steps[i] = orm.JSONKey(k)
	}

	return steps
}
