// Package render (this file) extends the package with expression
// projection: a SELECT whose output columns are aliased scalar expressions
// (function/CASE/column leaves, optionally a scalar subquery) rather than the
// entity's plain columns. See the projection builder for the public builder whose
// projected query renders through ProjectedSelect.
package render

import (
	"errors"
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// Projection is one aliased scalar output column of a projected SELECT. Expr
// is an erased scalar-expression Node using the KindColumn, KindFunc,
// KindLit or KindSubquery kinds (rendered by renderScalar); Alias is the
// output column name, rendered as `AS "alias"` (or omitted entirely when
// empty). A Projection is built by the expression builder's As combinators and never
// carries a raw caller-supplied SQL fragment.
type Projection struct {
	Expr  Node
	Alias string
}

// selectExpr renders p as it appears in a SELECT list: the scalar expression
// followed by its optional `AS "alias"`. It continues counter for any bound
// expression arguments (a COALESCE fallback, a scalar subquery's bound
// WHERE values), returning them in text order. scope carries the enclosing
// statement's table facts so a projected correlated subquery resolves its
// marker against the projected query's own FROM table.
func (p Projection) selectExpr(d dialect.Dialect, counter *argCounter, scope renderScope) (string, []any, error) {
	text, args, err := renderScalar(d, p.Expr, counter, scope)
	if err != nil {
		return "", nil, err
	}

	if p.Alias != "" {
		text += " AS " + d.QuoteIdent(p.Alias)
	}

	return text, args, nil
}

// ProjectedSelect renders a projected SELECT:
//
//	SELECT [DISTINCT ]<expr> AS "alias", ... FROM <table> [WHERE ...]
//	  [ORDER BY ...] [LIMIT <n>] [OFFSET <n>]
//
// and its positional arguments. projections are emitted in caller order (the
// order a caller scans each result row), and their expressions bind their
// arguments BEFORE the trailing WHERE/ORDER/LIMIT/OFFSET arguments -- matching
// their text order, so Postgres-style $N numbering stays sequential. err is
// non-nil for an empty projection list, a malformed projection expression, or
// an invalid modifier combination (see validateSelectModifiers) -- never a
// silently-broken statement.
func ProjectedSelect(
	d dialect.Dialect,
	table string,
	projections []Projection,
	where Node,
	order []OrderTerm,
	limit, offset int,
	mods SelectModifiers,
) (query string, args []any, err error) {
	if len(projections) == 0 {
		return "", nil, errors.New("orm/render: projected SELECT requires at least one projection")
	}

	err = validateSelectModifiers(d, mods)
	if err != nil {
		return "", nil, err
	}

	var b strings.Builder

	writeSelectPrefix(&b, d, mods)

	counter := &argCounter{}
	scope := renderScope{stmt: []string{table}}
	parts := make([]string, 0, len(projections))

	for _, p := range projections {
		text, pargs, exprErr := p.selectExpr(d, counter, scope)
		if exprErr != nil {
			return "", nil, exprErr
		}

		parts = append(parts, text)
		args = append(args, pargs...)
	}

	b.WriteString(strings.Join(parts, ", "))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(table))
	b.WriteString(tablesampleClause(mods.Tablesample))

	tailArgs, err := writeSelectTail(&b, d, where, order, limit, offset, counter)
	if err != nil {
		return "", nil, err
	}

	args = append(args, tailArgs...)

	b.WriteString(lockClause(d, mods))

	return b.String(), args, nil
}
