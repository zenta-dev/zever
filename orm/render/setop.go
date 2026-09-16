// Package render (this file) extends the package with set-operation
// (UNION/UNION ALL/INTERSECT/EXCEPT) rendering for the set-operation builder, plus
// the SHARED keyword/body pieces render/cte.go's recursive-CTE body
// rendering reuses (a recursive CTE body is `base UNION ALL recursive`, so
// the two files share SetOpOp's keyword rendering).
package render

import (
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// SetOpOp mirrors the builder's set-operation kind values (same underlying int
// representation), so a caller-side `render.SetOpOp(s.op)` plain conversion
// is always legal.
type SetOpOp int

// Supported set operators.
const (
	SetOpUnion SetOpOp = iota
	SetOpUnionAll
	SetOpIntersect
	SetOpExcept
)

func (o SetOpOp) keyword() string {
	switch o {
	case SetOpUnion:
		return "UNION"
	case SetOpUnionAll:
		return "UNION ALL"
	case SetOpIntersect:
		return "INTERSECT"
	case SetOpExcept:
		return "EXCEPT"
	default:
		return "UNION"
	}
}

// SetOp renders a set operation over two same-shaped SELECTs and its
// positional arguments:
//
//	SELECT <leftColumns> FROM <leftTable> [WHERE <leftWhere>]
//	  <op>
//	SELECT <rightColumns> FROM <rightTable> [WHERE <rightWhere>]
//	  [ORDER BY <order>] [LIMIT <n>] [OFFSET <n>]
//
// left and right share one ordered positional-argument list: leftWhere's
// placeholders/args always come first (lowest numbers), rightWhere's
// follow, then limit/offset -- matching the order the clauses appear in the
// rendered SQL text (matters for Postgres's $N-numbered placeholders).
//
// order applies to the set-operation RESULT and therefore references
// output column names (bare, never table-qualified) -- callers pass
// unqualified OrderTerms. limit/offset likewise apply to the whole result.
func SetOp(
	d dialect.Dialect,
	op SetOpOp,
	leftTable string, leftColumns []string, leftWhere Node,
	rightTable string, rightColumns []string, rightWhere Node,
	order []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	counter := &argCounter{}

	var b strings.Builder

	b.WriteString("SELECT ")
	b.WriteString(joinQuoted(d, leftColumns))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(leftTable))

	clause, whereArgs, err := renderExpr(d, leftWhere, counter)
	if err != nil {
		return "", nil, err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	args = append(args, whereArgs...)

	b.WriteString(" ")
	b.WriteString(op.keyword())
	b.WriteString(" SELECT ")
	b.WriteString(joinQuoted(d, rightColumns))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(rightTable))

	clause, whereArgs, err = renderExpr(d, rightWhere, counter)
	if err != nil {
		return "", nil, err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	args = append(args, whereArgs...)

	if len(order) > 0 {
		b.WriteString(" ORDER BY ")

		orderText, orderArgs, err := joinOrderBy(d, order, counter)
		if err != nil {
			return "", nil, err
		}

		b.WriteString(orderText)

		args = append(args, orderArgs...)
	}

	if limit > 0 {
		b.WriteString(" LIMIT ")
		b.WriteString(d.Placeholder(counter.next()))

		args = append(args, limit)
	}

	if offset > 0 {
		b.WriteString(" OFFSET ")
		b.WriteString(d.Placeholder(counter.next()))

		args = append(args, offset)
	}

	return b.String(), args, nil
}
