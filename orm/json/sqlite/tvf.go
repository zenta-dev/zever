// Package sqlite (this file) exposes SQLite's json1 table-valued functions
// `json_each(doc[, path])` and `json_tree(doc[, path])` as join sources --
// the SQLite half of orm's FROM-source abstraction (see orm.TableValuedSource
// and orm.JoinTVF). Unlike the render-only JSON_TABLE helper, a Source value
// composes into a real query: join it with orm.JoinTVF/LeftJoinTVF, filter
// and order by its typed column handles, and scan each source row into the
// ready-made EachRow (or a caller's own row type).
//
// Every supported dialect treats a FROM-clause table-valued function as
// implicitly LATERAL, so the function argument may reference a column of the
// table it is joined to (e.g. `json_each("widgets"."tags")`) with no LATERAL
// keyword. The source is gated by dialect.JSONEachDialect: MySQL and Postgres
// return a typed dialect.ErrUnsupportedByDialect, never invalid SQL.
package sqlite

import (
	"fmt"
	"strings"

	orm "github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
)

// jsonEachColumns is the fixed output column list of json_each/json_tree, in
// the order SQLite documents and in the order EachRow.Scan reads them.
var jsonEachColumns = []string{"key", "value", "type", "atom", "id", "parent", "fullkey", "path", "root"}

// Source is a typed SQLite json1 table-valued function source over a JSON
// column of entity T. B is the row type the source's columns scan into; use
// EachRow (or TreeRow) for the ready-made shape, or a caller struct whose
// Scan reads exactly SrcColumns() in order.
//
// The exported column handles are typed references to the source's output
// columns, qualified to the source alias, so WHERE/ORDER BY over the source
// composes with the existing predicate machinery:
//
//	je := sqlite.JSONEach[EachRow](Docs.Tags, "je")
//	rows, err := orm.JoinTVF(orm.From(Docs), je).
//	 WhereSource(je.Value.Eq("go")).
//	 OrderBySource(je.Key.Asc()).
//	 All(ctx, conn)
type Source[B any] struct {
	alias   string
	fn      string
	table   string
	col     string
	jsonpat string
	hasPath bool

	// Typed column handles, qualified to the source alias.
	Key     orm.NullableColumn[B, any]
	Value   orm.NullableColumn[B, any]
	Type    orm.Column[B, string]
	Atom    orm.NullableColumn[B, any]
	ID      orm.Column[B, int64]
	Parent  orm.NullableColumn[B, int64]
	Fullkey orm.Column[B, string]
	Path    orm.Column[B, string]
	Root    orm.NullableColumn[B, any]
}

// JSONEach roots a json_each(doc) source over the JSON column c, joined under
// alias. The column's own table name qualifies the function argument, so the
// source must be joined to a query whose FROM table is that same table.
func JSONEach[B any, T any, V any](c orm.Column[T, V], alias string) Source[B] {
	return newSource[B](c, "json_each", "", false, alias)
}

// JSONEachPath roots a json_each(doc, '$.path') source, restricting the
// iteration to the JSON array/object at path.
func JSONEachPath[B any, T any, V any](c orm.Column[T, V], path, alias string) Source[B] {
	return newSource[B](c, "json_each", path, true, alias)
}

// JSONTree roots a json_tree(doc) source over the JSON column c, joined under
// alias. json_tree recurses into the whole document, emitting one row per
// node (unlike json_each, which is shallow).
func JSONTree[B any, T any, V any](c orm.Column[T, V], alias string) Source[B] {
	return newSource[B](c, "json_tree", "", false, alias)
}

// JSONTreePath roots a json_tree(doc, '$.path') source, starting the
// recursion at path.
func JSONTreePath[B any, T any, V any](c orm.Column[T, V], path, alias string) Source[B] {
	return newSource[B](c, "json_tree", path, true, alias)
}

func newSource[B any, T any, V any](c orm.Column[T, V], fn, path string, hasPath bool, alias string) Source[B] {
	return Source[B]{
		alias:   alias,
		fn:      fn,
		table:   c.Table(),
		col:     c.Name(),
		jsonpat: path,
		hasPath: hasPath,
		Key:     orm.NewNullableColumn[B, any](alias, "key"),
		Value:   orm.NewNullableColumn[B, any](alias, "value"),
		Type:    orm.NewColumn[B, string](alias, "type"),
		Atom:    orm.NewNullableColumn[B, any](alias, "atom"),
		ID:      orm.NewColumn[B, int64](alias, "id"),
		Parent:  orm.NewNullableColumn[B, int64](alias, "parent"),
		Fullkey: orm.NewColumn[B, string](alias, "fullkey"),
		Path:    orm.NewColumn[B, string](alias, "path"),
		Root:    orm.NewNullableColumn[B, any](alias, "root"),
	}
}

// SrcAlias returns the derived-table alias. See orm.TableValuedSource.
func (s Source[B]) SrcAlias() string { return s.alias }

// NewRow returns a fresh, zero B. See orm.TableValuedSource.
func (Source[B]) NewRow() B {
	var z B

	return z
}

// SrcColumns returns json1's fixed output column list in Scan order. See
// orm.TableValuedSource.
func (s Source[B]) SrcColumns() []string {
	cp := make([]string, len(jsonEachColumns))
	copy(cp, jsonEachColumns)

	return cp
}

// RenderSource renders `json_each("table"."col"[, '$.path'])` (or json_tree),
// gated by dialect.JSONEachDialect. A dialect without the capability --
// MySQL and Postgres -- returns a typed dialect.ErrUnsupportedByDialect,
// never invalid SQL. It binds no arguments.
func (s Source[B]) RenderSource(d dialect.Dialect) (string, error) {
	jd, ok := d.(dialect.JSONEachDialect)
	if !ok || !jd.SupportsJSONEach() {
		return "", fmt.Errorf("orm/json/sqlite: %w: dialect %q has no json_each/json_tree", dialect.ErrUnsupportedByDialect, d.Name())
	}

	if s.table == "" || s.col == "" {
		return "", fmt.Errorf("orm/json/sqlite: %s: empty document column", s.fn)
	}

	var b strings.Builder

	b.WriteString(s.fn)
	b.WriteString("(")
	b.WriteString(d.QuoteIdent(s.table))
	b.WriteString(".")
	b.WriteString(d.QuoteIdent(s.col))

	if s.hasPath {
		b.WriteString(", ")
		b.WriteString(sqlStringLiteral(s.jsonpat))
	}

	b.WriteString(")")

	return b.String(), nil
}

// EachRow is the ready-made row type for a json_each/json_tree source: one
// field per documented json1 column, in SrcColumns order. The dynamically
// typed columns (key/value/atom/root) are `any`, since json1 returns an
// integer, text, or NULL depending on the JSON value; type/fullkey/path are
// text and id/parent are integers.
type EachRow struct {
	Key     any
	Value   any
	Type    string
	Atom    any
	ID      int64
	Parent  orm.Option[int64]
	Fullkey string
	Path    string
	Root    any
}

// Scan reads one JSON TVF row positionally, matching SrcColumns order.
func (r *EachRow) Scan(row orm.Row) error {
	return row.Scan(&r.Key, &r.Value, &r.Type, &r.Atom, &r.ID, &r.Parent, &r.Fullkey, &r.Path, &r.Root)
}

// TreeRow is the json_tree row shape; it is identical to EachRow.
type TreeRow = EachRow

// sqlStringLiteral wraps s in single quotes, doubling embedded quotes -- the
// standard SQL string-literal escaping for a json path argument. The path is
// data, never SQL structure.
func sqlStringLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
