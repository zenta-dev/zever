// Package sqlite provides typed helpers over SQLite's json1 functions
// (`json_extract`, `json_type`, `json_each`), built as orm predicates that
// compose into orm.From(...).Where(...) exactly like Column methods do:
//
//	sqlite.ExtractText(col, "name").Eq("alice") // json_extract(col,'$.name') = ?
//	sqlite.KeyExists(col, "meta") // json_extract(col,'$.meta') IS NOT NULL
//	sqlite.ArrayContains(col, "go") // EXISTS (SELECT 1 FROM json_each(col) WHERE value = ?)
//
// This package is the sqlite half of the "same typed call, dialect-specific
// renderer" promise (): its surface reads
// near-identically to orm/json/postgres, and a predicate built through
// either package renders correctly on whichever dialect the query executes
// on. The syntax families genuinely differ -- postgres uses jsonb
// operators, sqlite uses json1 functions, mysql uses JSON_* functions --
// and the surface is honest about it: postgres's `Contains` (`@>`) has no
// sqlite equivalent (json1 has no containment operator), so this package
// exposes ArrayContains (json_each) instead.
package sqlite

import (
	orm "github.com/zenta-dev/zever/orm"
)

// Path is a JSON path expression over entity T: a base JSON column plus an
// optional chain of extraction steps. On sqlite every extraction renders as
// a single json_extract(col, '$.a.b') call -- the steps accumulate into one
// json path argument.
type Path[T any] struct {
	table string
	col   string
	steps []orm.JSONStep
}

// JSONPath roots a JSON path at a JSON column of entity T.
func JSONPath[T any, V any](c orm.Column[T, V]) Path[T] {
	return Path[T]{table: c.Table(), col: c.Name()}
}

// Extract appends an object-key extraction step
// (`json_extract(col, '$.a.b')`).
func (p Path[T]) Extract(key string) Path[T] {
	p.steps = append(append([]orm.JSONStep(nil), p.steps...), orm.JSONKey(key))

	return p
}

// ExtractIndex appends an array-index extraction step
// (`json_extract(col, '$.a[0]')`).
func (p Path[T]) ExtractIndex(i int64) Path[T] {
	p.steps = append(append([]orm.JSONStep(nil), p.steps...), orm.JSONIndex(i))

	return p
}

// PathExtract replaces the path with a full multi-step extraction
// (`json_extract(col, '$.a.b')`).
func (p Path[T]) PathExtract(keys ...string) Path[T] {
	p.steps = jsonKeys(keys)

	return p
}

// ExtractText ends the path as a text extraction
// (`json_extract(col, '$.key')`).
func (p Path[T]) ExtractText(key string) Text[T] {
	steps := append(append([]orm.JSONStep(nil), p.steps...), orm.JSONKey(key))

	return Text[T]{table: p.table, col: p.col, steps: steps}
}

// ExtractIndexText ends the path as a text extraction of an array element
// (`json_extract(col, '$.key[0]')`).
func (p Path[T]) ExtractIndexText(i int64) Text[T] {
	steps := append(append([]orm.JSONStep(nil), p.steps...), orm.JSONIndex(i))

	return Text[T]{table: p.table, col: p.col, steps: steps}
}

// PathExtractText ends the path as a text extraction
// (`json_extract(col, '$.a.b')`).
func (p Path[T]) PathExtractText(keys ...string) Text[T] {
	return Text[T]{table: p.table, col: p.col, steps: jsonKeys(keys)}
}

// KeyExists builds a key-existence predicate, rendered as
// `json_extract(col, '$.key') IS NOT NULL` (sqlite json1 has no `?`
// operator).
func (p Path[T]) KeyExists(key string) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONKeyExists, Steps: p.steps}, orm.Eq, key)
}

// ArrayContains builds an array-membership predicate, rendered as
// `EXISTS (SELECT 1 FROM json_each(col, '$.path') WHERE value = ?)`: does
// the JSON array at the path contain the given element?
func (p Path[T]) ArrayContains(elem any) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONArrayContains, Steps: p.steps}, orm.Eq, elem)
}

// Eq builds a `<extract> = v` predicate, comparing the extracted scalar
// against a Go string (json1 returns text for scalar JSON values).
func (p Path[T]) Eq(v string) orm.Predicate[T] { return p.cmp(orm.Eq, v) }

// Neq builds a `<extract> != v` predicate.
func (p Path[T]) Neq(v string) orm.Predicate[T] { return p.cmp(orm.Neq, v) }

// IsNull builds a `<extract> IS NULL` predicate (true when the path does
// not exist or holds a JSON null).
func (p Path[T]) IsNull() orm.Predicate[T] { return p.cmp(orm.IsNull, nil) }

// IsNotNull builds a `<extract> IS NOT NULL` predicate.
func (p Path[T]) IsNotNull() orm.Predicate[T] { return p.cmp(orm.IsNotNull, nil) }

func (p Path[T]) cmp(op orm.Op, value any) orm.Predicate[T] {
	return orm.NewJSONPredicate[T](p.table, p.col, orm.JSONExpr{Op: orm.JSONExtract, Steps: p.steps}, op, value)
}

// Type builds a `json_type(col, '$.path')` expression, comparable against
// the json1 type name ("object", "array", "integer", "real", "text",
// "true", "false", "null").
func (p Path[T]) Type() Text[T] {
	return Text[T]{table: p.table, col: p.col, steps: p.steps, op: orm.JSONType}
}

// Text is a text-valued JSON extraction over entity T: the result of
// json_extract or json_type, comparable against Go strings.
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

// Extract builds a json key extraction at a column
// (`json_extract(col, '$.key')`).
func Extract[T any, V any](c orm.Column[T, V], key string) Path[T] {
	return JSONPath(c).Extract(key)
}

// ExtractIndex builds a json array-index extraction
// (`json_extract(col, '$[0]')`).
func ExtractIndex[T any, V any](c orm.Column[T, V], i int64) Path[T] {
	return JSONPath(c).ExtractIndex(i)
}

// ExtractText builds a text key extraction (`json_extract(col, '$.key')`).
func ExtractText[T any, V any](c orm.Column[T, V], key string) Text[T] {
	return JSONPath(c).ExtractText(key)
}

// ExtractIndexText builds a text array-index extraction
// (`json_extract(col, '$.key[0]')`).
func ExtractIndexText[T any, V any](c orm.Column[T, V], i int64) Text[T] {
	return JSONPath(c).ExtractIndexText(i)
}

// PathExtract builds a multi-step extraction at a column
// (`json_extract(col, '$.a.b')`).
func PathExtract[T any, V any](c orm.Column[T, V], keys ...string) Path[T] {
	return JSONPath(c).PathExtract(keys...)
}

// PathExtractText builds a multi-step text extraction at a column
// (`json_extract(col, '$.a.b')`).
func PathExtractText[T any, V any](c orm.Column[T, V], keys ...string) Text[T] {
	return JSONPath(c).PathExtractText(keys...)
}

// KeyExists builds a key-existence predicate at a column
// (`json_extract(col, '$.key') IS NOT NULL`).
func KeyExists[T any, V any](c orm.Column[T, V], key string) orm.Predicate[T] {
	return JSONPath(c).KeyExists(key)
}

// ArrayContains builds an array-membership predicate at a column
// (`EXISTS (SELECT 1 FROM json_each(col) WHERE value = ?)`).
func ArrayContains[T any, V any](c orm.Column[T, V], elem any) orm.Predicate[T] {
	return JSONPath(c).ArrayContains(elem)
}

// Type builds a `json_type(col)` expression for the whole document,
// comparable against the json1 type name. For a nested value's type use
// JSONPath(col).Extract(key).Type().
func Type[T any, V any](c orm.Column[T, V]) Text[T] {
	return JSONPath(c).Type()
}

func jsonKeys(keys []string) []orm.JSONStep {
	steps := make([]orm.JSONStep, len(keys))
	for i, k := range keys {
		steps[i] = orm.JSONKey(k)
	}

	return steps
}
