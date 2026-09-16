package render

import (
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// SelectTVF renders a single projecting SELECT over a table-valued function
// source: an explicit qualified column list from BOTH the left table and the
// source (never `SELECT *`), the `CROSS JOIN <source> AS <alias>` (leftJoin
// false) or `LEFT JOIN <source> AS <alias> ON TRUE` (leftJoin true) clause,
// a WHERE clause combining whereLeft (qualified to leftTable) and
// whereSource (qualified to sourceAlias), an optional per-side ORDER BY, and
// LIMIT/OFFSET. It is the render behind the JoinTVF/LeftJoinTVF builders --
// the table-valued-function analog of SelectLateral, minus the LATERAL
// keyword: every supported dialect correlates a FROM-clause table-valued
// function to preceding FROM entries implicitly, so no LATERAL is emitted.
//
// sourceSQL is the already-rendered function call (no alias), so the source's
// own dialect package owns its capability gate and syntax; render only places
// it in the FROM clause and tags it with sourceAlias. sourceColumns are the
// source's output column names, in the order the row type's Scan reads them.
// The returned args come only from whereLeft/whereSource (a TVF binds none),
// numbered in emitted-SQL order.
func SelectTVF(
	d dialect.Dialect,
	leftJoin bool,
	leftTable string, leftColumns []string,
	sourceSQL, sourceAlias string, sourceColumns []string,
	whereLeft, whereSource Node,
	orderLeft, orderSource []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	var b strings.Builder

	b.WriteString("SELECT ")
	b.WriteString(joinQualifiedColumns(d, []string{leftTable, sourceAlias}, [][]string{leftColumns, sourceColumns}))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(leftTable))
	b.WriteString(" ")

	if leftJoin {
		b.WriteString("LEFT JOIN ")
	} else {
		b.WriteString("CROSS JOIN ")
	}

	b.WriteString(sourceSQL)
	b.WriteString(" AS ")
	b.WriteString(d.QuoteIdent(sourceAlias))

	if leftJoin {
		b.WriteString(" ON TRUE")
	}

	counter := &argCounter{}

	combined := combineWhere(qualifyNode(whereLeft, ""), qualifyNode(whereSource, ""))

	// The source join is itself a multi-table enclosing statement: a subquery
	// nested in its WHERE correlates against BOTH the left table and the
	// source alias, so a marker may reference either side.
	clause, whereArgs, err := renderExprCorr(d, combined, counter, renderScope{stmt: []string{leftTable, sourceAlias}})
	if err != nil {
		return "", nil, err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	args = append(args, whereArgs...)

	if len(orderLeft) > 0 || len(orderSource) > 0 {
		b.WriteString(" ORDER BY ")

		orderText, orderArgs, err := joinOrderByAll(d, []string{leftTable, sourceAlias}, [][]OrderTerm{orderLeft, orderSource}, counter)
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
