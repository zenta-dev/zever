// Package postgres (this file) exposes Postgres's set-returning jsonb
// functions `jsonb_array_elements(doc)` and `jsonb_array_elements_text(doc)`
// as join sources -- the Postgres half of orm's FROM-source abstraction (see
// orm.TableValuedSource and orm.JoinTVF). A Postgres FROM-clause
// set-returning function is implicitly LATERAL and may reference a column of
// an earlier FROM entry, so `jsonb_array_elements("docs"."data")` correlates
// to the joined table with no LATERAL keyword.
//
// Both functions emit a single output column named `value` (jsonb for
// jsonb_array_elements, text for jsonb_array_elements_text). The source is
// gated by dialect.JSONSetReturningDialect: MySQL and SQLite return a typed
// dialect.ErrUnsupportedByDialect.
//
// jsonb_path_query is intentionally NOT exposed as a source here: unlike
// jsonb_array_elements, its result column is named after the function
// (`jsonb_path_query`), and a faithful source would need a column-alias list
// on the derived table (`AS alias(value)`) that the shared render path does
// not emit -- so it is dropped rather than shipped with a misleading column
// name. The single-argument array functions cover the correlated-explode use
// case cleanly.
package postgres

import (
	"fmt"

	orm "github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/orm/dialect"
)

// Source is a typed Postgres set-returning JSON source over the JSON column
// of entity T. B is the row type the source's single `value` column scans
// into; use ElemRow for the ready shape, or a caller struct whose Scan reads
// one value.
type Source[B any] struct {
	alias string
	fn    string
	table string
	col   string

	// Value is the typed handle for the source's output column, qualified to
	// the source alias.
	Value orm.NullableColumn[B, any]
}

// ArrayElements roots a `jsonb_array_elements(doc)` source: one jsonb row per
// array element of the column c, joined under alias.
func ArrayElements[B any, T any, V any](c orm.Column[T, V], alias string) Source[B] {
	return newSource[B](c, "jsonb_array_elements", alias)
}

// ArrayElementsText roots a `jsonb_array_elements_text(doc)` source: one text
// row per array element. It differs from ArrayElements only in the value's
// SQL type.
func ArrayElementsText[B any, T any, V any](c orm.Column[T, V], alias string) Source[B] {
	return newSource[B](c, "jsonb_array_elements_text", alias)
}

func newSource[B any, T any, V any](c orm.Column[T, V], fn, alias string) Source[B] {
	return Source[B]{
		alias: alias,
		fn:    fn,
		table: c.Table(),
		col:   c.Name(),
		Value: orm.NewNullableColumn[B, any](alias, "value"),
	}
}

// SrcAlias returns the derived-table alias. See orm.TableValuedSource.
func (s Source[B]) SrcAlias() string { return s.alias }

// NewRow returns a fresh, zero B. See orm.TableValuedSource.
func (Source[B]) NewRow() B {
	var z B

	return z
}

// SrcColumns returns the source's single output column, `value`. See
// orm.TableValuedSource.
func (Source[B]) SrcColumns() []string { return []string{"value"} }

// RenderSource renders `jsonb_array_elements("table"."col")` (or the _text
// variant), gated by dialect.JSONSetReturningDialect. A dialect without the
// capability -- MySQL and SQLite -- returns a typed
// dialect.ErrUnsupportedByDialect, never invalid SQL. It binds no arguments.
func (s Source[B]) RenderSource(d dialect.Dialect) (string, error) {
	jd, ok := d.(dialect.JSONSetReturningDialect)
	if !ok || !jd.SupportsJSONSetReturning() {
		return "", fmt.Errorf("orm/json/postgres: %w: dialect %q has no set-returning JSON function", dialect.ErrUnsupportedByDialect, d.Name())
	}

	if s.table == "" || s.col == "" {
		return "", fmt.Errorf("orm/json/postgres: %s: empty document column", s.fn)
	}

	return s.fn + "(" + d.QuoteIdent(s.table) + "." + d.QuoteIdent(s.col) + ")", nil
}

// ElemRow is the ready-made row type for an ArrayElements/ArrayElementsText
// source: a single `value` field. It is `any` because the SQL type depends on
// the function variant (jsonb vs text), and a jsonb value scans as []byte or
// string depending on the driver.
type ElemRow struct {
	Value any
}

// Scan reads the source's single value column.
func (r *ElemRow) Scan(row orm.Row) error { return row.Scan(&r.Value) }
