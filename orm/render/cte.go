// Package render (this file) extends the package with CTE (`WITH` /
// `WITH RECURSIVE`) statement rendering for the CTE builder. A CTE wraps a
// named SELECT body -- which may be a plain single-table SELECT (a
// Query[T,PT]), a projecting join (a Join2), a grouped aggregate (a
// GroupedQuery[T]), or a set operation (a recursive CTE's `base UNION ALL
// recursive` shape) -- into
//
//	WITH [RECURSIVE] "name" AS [MATERIALIZED] (<body>)
//	  [SEARCH DEPTH FIRST BY <cols> SET <ordcol>] [CYCLE <cols> SET <cyc> USING <path>]
//	  <outer SELECT>
//
// The body and the outer SELECT share ONE argCounter, so placeholder
// numbering stays sequential across the whole statement (matters for
// Postgres's $N-numbered placeholders). the CTE builder converts its typed
// Query/Join2/GroupedQuery values into this erased CTEBody shape with plain
// field-for-field literals -- the same pattern the query builder's
// toRenderNode/toRenderOrder already use.
package render

import (
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// CTEBodyKind discriminates the shape of a CTE body SELECT.
type CTEBodyKind int

// Supported CTE body kinds.
const (
	// CTEBodyPlain is a single-table SELECT (a Query[T,PT]), optionally
	// constrained by a filter-only JOIN to one other table (the recursive
	// branch of a recursive CTE, which joins back to the CTE itself).
	CTEBodyPlain CTEBodyKind = iota
	// CTEBodyJoin is a projecting two-table join (a Join2).
	CTEBodyJoin
	// CTEBodyGrouped is a GROUP BY/aggregate SELECT (a GroupedQuery[T]).
	CTEBodyGrouped
	// CTEBodySetOp is a set operation over two sub-bodies (the `base UNION
	// ALL recursive` shape of a recursive CTE).
	CTEBodySetOp
)

// CTEMaterialization selects the optional `MATERIALIZED` / `NOT MATERIALIZED`
// hint on a CTE definition. The zero value emits no hint, so a CTEBody built
// before this field existed renders byte-for-byte as before.
type CTEMaterialization int

// Supported CTE materialization hints.
const (
	// CTEMaterializeDefault emits no hint: `WITH name AS (...)`.
	CTEMaterializeDefault CTEMaterialization = iota
	// CTEMaterializeAlways emits `WITH name AS MATERIALIZED (...)`.
	CTEMaterializeAlways
	// CTEMaterializeNever emits `WITH name AS NOT MATERIALIZED (...)`.
	CTEMaterializeNever
)

// CTEBody is the erased shape of one CTE body SELECT. Its field groups
// mirror exactly the parameter lists of Select / SelectJoin /
// GroupedSelect, plus the SetOp shape and a filter-only-join extension --
// see each kind's comment on CTEBodyKind for which fields it uses. Only the
// fields a kind needs are ever read; the rest stay zero.
type CTEBody struct {
	Kind    CTEBodyKind
	Table   string
	Columns []string
	Where   Node
	Order   []OrderTerm
	Limit   int
	Offset  int

	// JoinFilter (used by CTEBodyPlain): constrain the FROM with
	// `JOIN <JoinTable> ON <Table>.<ParentCol> = <JoinTable>.<ChildCol>`
	// before the WHERE clause. ParentCol is on Table's side; ChildCol on
	// JoinTable's.
	JoinTable string
	ParentCol string
	ChildCol  string

	// Join fields (used by CTEBodyJoin): the two sides of a projecting
	// join. ParentCol/ChildCol double as the equi-join keys here. OrderRight
	// is the right table's ORDER BY list, qualified to RightTable -- the
	// per-table ORDER BY split SelectJoin already implements, so a Join2
	// carrying OrderByRight survives its CTE body.
	JoinType     JoinType
	RightTable   string
	RightColumns []string
	WhereRight   Node
	OrderRight   []OrderTerm

	// Grouped fields (used by CTEBodyGrouped).
	GroupCols []string
	Aggs      []Aggregate
	Having    HavingNode

	// SetOp fields (used by CTEBodySetOp).
	SetOp     SetOpOp
	LeftBody  *CTEBody
	RightBody *CTEBody

	// Materialization selects the optional `[NOT] MATERIALIZED` hint on the
	// CTE definition. It is read from the TOP-LEVEL body only (the one passed
	// to SelectWith/CountWith), so a recursive CTE's set-op body carries it
	// just as a plain body does.
	Materialization CTEMaterialization

	// Search/Cycle fields (read from the TOP-LEVEL body only): the optional
	// recursive-CTE clauses rendered after the body's closing paren --
	// `SEARCH DEPTH FIRST BY <SearchBy> SET <SearchSet>` and
	// `CYCLE <CycleBy> SET <CycleSet> USING <CycleUsing>`. SearchBy/CycleBy
	// are output column names of the CTE; SearchSet/CycleSet/CycleUsing are
	// the new column names the clauses introduce. Empty search/cycle fields
	// render nothing.
	SearchBy   []string
	SearchSet  string
	CycleBy    []string
	CycleSet   string
	CycleUsing string
}

// SelectWith renders a whole
//
//	WITH [RECURSIVE] "name" AS [MATERIALIZED] (<body>)
//	  [SEARCH DEPTH FIRST BY <cols> SET <ordcol>] [CYCLE <cols> SET <cyc> USING <path>]
//	  SELECT <outerColumns> FROM "name"
//	  [WHERE <outerWhere>] [ORDER BY <outerOrder>] [LIMIT <n>] [OFFSET <n>]
//
// statement and its positional arguments. body's placeholders/args always
// come first (they appear earliest in the SQL text), then outerWhere's,
// then limit/offset -- all numbered by one shared argCounter.
//
// outerWhere/outerOrder apply to the outer SELECT, whose FROM is only the
// CTE, so their column references must be the CTE's own (unqualified)
// output column names; the renderer emits them as-is. err is non-nil only
// when body is a Grouped body whose HAVING leaf fails the numeric
// type-check (see render/agg.go's renderHavingLeaf) -- a rendering-time
// error, never a panic.
func SelectWith(
	d dialect.Dialect,
	name string,
	recursive bool,
	body CTEBody,
	outerColumns []string,
	outerWhere Node,
	outerOrder []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	var b strings.Builder

	b.WriteString("WITH ")

	if recursive {
		b.WriteString("RECURSIVE ")
	}

	b.WriteString(d.QuoteIdent(name))
	b.WriteString(" AS ")
	writeMaterialization(&b, body.Materialization)
	b.WriteString("(")

	counter := &argCounter{}

	args, err = writeCTEBody(&b, d, body, counter)
	if err != nil {
		return "", nil, err
	}

	b.WriteString(")")
	writeSearchCycle(&b, d, body)

	b.WriteString(" SELECT ")
	b.WriteString(joinQuoted(d, outerColumns))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(name))

	outerTail, err := writeSelectTail(&b, d, outerWhere, outerOrder, limit, offset, counter)
	if err != nil {
		return "", nil, err
	}

	args = append(args, outerTail...)

	return b.String(), args, nil
}

// CountWith renders `WITH [RECURSIVE] "name" AS [MATERIALIZED] (<body>)
// [SEARCH ...] [CYCLE ...] SELECT COUNT(*) FROM "name"` and its positional
// arguments -- the COUNT analog of
// SelectWith, used by CTEQuery[T,PT].Count (order/limit/offset are
// meaningless for a count and are never rendered). err is non-nil under
// the same conditions SelectWith's is.
func CountWith(d dialect.Dialect, name string, recursive bool, body CTEBody) (query string, args []any, err error) {
	var b strings.Builder

	b.WriteString("WITH ")

	if recursive {
		b.WriteString("RECURSIVE ")
	}

	b.WriteString(d.QuoteIdent(name))
	b.WriteString(" AS ")
	writeMaterialization(&b, body.Materialization)
	b.WriteString("(")

	counter := &argCounter{}

	args, err = writeCTEBody(&b, d, body, counter)
	if err != nil {
		return "", nil, err
	}

	b.WriteString(")")
	writeSearchCycle(&b, d, body)

	b.WriteString(" SELECT COUNT(*) FROM ")
	b.WriteString(d.QuoteIdent(name))

	return b.String(), args, nil
}

// writeMaterialization writes the optional `MATERIALIZED ` / `NOT MATERIALIZED `
// fragment between `AS ` and the body's opening paren. The default enum value
// writes nothing, so a CTE without a hint renders exactly as before.
func writeMaterialization(b *strings.Builder, m CTEMaterialization) {
	if m == CTEMaterializeAlways {
		b.WriteString("MATERIALIZED ")
	}

	if m == CTEMaterializeNever {
		b.WriteString("NOT MATERIALIZED ")
	}
}

// writeSearchCycle writes the optional recursive-CTE `SEARCH` / `CYCLE`
// clauses after the body's closing paren. A clause is emitted only when its
// BY column list is non-empty, so an unset CTEBody renders nothing. Both
// clause column lists and the introduced SET/USING column names are quoted
// identifiers.
func writeSearchCycle(b *strings.Builder, d dialect.Dialect, body CTEBody) {
	if len(body.SearchBy) > 0 {
		b.WriteString(" SEARCH DEPTH FIRST BY ")
		b.WriteString(joinQuoted(d, body.SearchBy))
		b.WriteString(" SET ")
		b.WriteString(d.QuoteIdent(body.SearchSet))
	}

	if len(body.CycleBy) > 0 {
		b.WriteString(" CYCLE ")
		b.WriteString(joinQuoted(d, body.CycleBy))
		b.WriteString(" SET ")
		b.WriteString(d.QuoteIdent(body.CycleSet))
		b.WriteString(" USING ")
		b.WriteString(d.QuoteIdent(body.CycleUsing))
	}
}

// writeCTEBody renders one CTEBody's SELECT text into b, numbering
// placeholders with counter and returning the bound arguments. A SetOp body
// renders `SELECT ... <op> SELECT ...` with each side sharing counter; a
// side's own order/limit/offset are dropped (a bare ORDER BY/LIMIT inside a
// set-operation operand is not legal SQL without parenthesization, and is
// meaningless for a recursive CTE's base/recursive branches).
func writeCTEBody(b *strings.Builder, d dialect.Dialect, body CTEBody, counter *argCounter) ([]any, error) {
	switch body.Kind {
	case CTEBodyPlain:
		return writePlainBody(b, d, body, counter)
	case CTEBodyJoin:
		return writeJoinBody(b, d, body, counter)
	case CTEBodyGrouped:
		return writeGroupedBody(b, d, body, counter)
	case CTEBodySetOp:
		if body.LeftBody == nil || body.RightBody == nil {
			return nil, nil
		}

		left, right := stripSetOpOperand(*body.LeftBody), stripSetOpOperand(*body.RightBody)

		var args []any

		leftArgs, err := writeCTEBody(b, d, left, counter)
		if err != nil {
			return nil, err
		}

		args = append(args, leftArgs...)

		b.WriteString(" ")
		b.WriteString(body.SetOp.keyword())
		b.WriteString(" ")

		rightArgs, err := writeCTEBody(b, d, right, counter)
		if err != nil {
			return nil, err
		}

		args = append(args, rightArgs...)

		return args, nil
	default:
		return nil, nil
	}
}

// stripSetOpOperand zeroes a set-operation operand's order/limit/offset so
// they can never leak into invalid SQL -- see writeCTEBody's SetOp case.
func stripSetOpOperand(body CTEBody) CTEBody {
	body.Order = nil
	body.Limit = 0
	body.Offset = 0

	return body
}

// writePlainBody renders a CTEBodyPlain body. When a JoinFilter is present
// the FROM has two tables (the body's own and the join target, which for a
// recursive CTE is the CTE being defined -- carrying the SAME column
// names), so the select list and WHERE clause are qualified to body.Table
// to keep every reference unambiguous. Without a JoinFilter the single
// table needs no qualification.
func writePlainBody(b *strings.Builder, d dialect.Dialect, body CTEBody, counter *argCounter) ([]any, error) {
	columns := body.Columns
	where := qualifyNode(body.Where, "")

	if body.JoinTable != "" {
		columns = qualifyColumns(body.Columns, body.Table)
	}

	b.WriteString("SELECT ")
	b.WriteString(joinQuoted(d, columns))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(body.Table))

	if body.JoinTable != "" {
		b.WriteString(" JOIN ")
		b.WriteString(d.QuoteIdent(body.JoinTable))
		b.WriteString(" ON ")
		b.WriteString(quoteColumn(d, body.Table+"."+body.ParentCol))
		b.WriteString(" = ")
		b.WriteString(quoteColumn(d, body.JoinTable+"."+body.ChildCol))
	}

	args, err := writeSelectTail(b, d, where, body.Order, body.Limit, body.Offset, counter)
	if err != nil {
		return nil, err
	}

	return args, nil
}

// qualifyColumns prefixes every column with table (for a multi-table FROM
// where bare column names would be ambiguous).
func qualifyColumns(columns []string, table string) []string {
	out := make([]string, len(columns))
	for i, c := range columns {
		out[i] = table + "." + c
	}

	return out
}

// writeJoinBody renders a CTEBodyJoin body -- the same explicit qualified
// column list / JOIN clause / qualified-WHERE shape as SelectJoin, with
// placeholders numbered by the shared counter. The ORDER BY renders the
// left and right tables' term lists separately, each qualified to its own
// table, exactly like SelectJoin's orderLeft/orderRight split.
func writeJoinBody(b *strings.Builder, d dialect.Dialect, body CTEBody, counter *argCounter) ([]any, error) {
	b.WriteString("SELECT ")
	b.WriteString(joinQualifiedColumns(d, []string{body.Table, body.RightTable}, [][]string{body.Columns, body.RightColumns}))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(body.Table))
	b.WriteString(" ")
	b.WriteString(body.JoinType.keyword())
	b.WriteString(" ")
	b.WriteString(d.QuoteIdent(body.RightTable))
	b.WriteString(" ON ")
	b.WriteString(quoteColumn(d, body.Table+"."+body.ParentCol))
	b.WriteString(" = ")
	b.WriteString(quoteColumn(d, body.RightTable+"."+body.ChildCol))

	combined := combineWhere(qualifyNode(body.Where, ""), qualifyNode(body.WhereRight, ""))

	var args []any

	// A CTE join body is the same multi-table statement shape SelectJoin
	// renders, so a subquery nested in its WHERE correlates against the
	// body's own two tables too.
	whereClause, whereArgs, err := renderExprCorr(d, combined, counter, renderScope{stmt: []string{body.Table, body.RightTable}})
	if err != nil {
		return nil, err
	}

	if whereClause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(whereClause)
	}

	args = append(args, whereArgs...)

	if len(body.Order) > 0 || len(body.OrderRight) > 0 {
		b.WriteString(" ORDER BY ")

		orderText, orderArgs, err := joinOrderByAll(d, []string{body.Table, body.RightTable}, [][]OrderTerm{body.Order, body.OrderRight}, counter)
		if err != nil {
			return nil, err
		}

		b.WriteString(orderText)

		args = append(args, orderArgs...)
	}

	if body.Limit > 0 {
		b.WriteString(" LIMIT ")
		b.WriteString(d.Placeholder(counter.next()))

		args = append(args, body.Limit)
	}

	if body.Offset > 0 {
		b.WriteString(" OFFSET ")
		b.WriteString(d.Placeholder(counter.next()))

		args = append(args, body.Offset)
	}

	return args, nil
}

// writeGroupedBody renders a CTEBodyGrouped body -- the same select list /
// WHERE / GROUP BY / HAVING shape as GroupedSelect, with placeholders
// numbered by the shared counter. The HAVING leaf type-check can fail (see
// render/agg.go's renderHavingLeaf), in which case a rendering-time error
// is returned rather than a panic.
func writeGroupedBody(b *strings.Builder, d dialect.Dialect, body CTEBody, counter *argCounter) ([]any, error) {
	parts := make([]string, 0, len(body.GroupCols)+len(body.Aggs))

	var args []any

	for _, c := range body.GroupCols {
		parts = append(parts, quoteColumn(d, c))
	}

	for _, a := range body.Aggs {
		text, aggArgs, err := a.selectExpr(d, counter)
		if err != nil {
			return nil, err
		}

		parts = append(parts, text)

		args = append(args, aggArgs...)
	}

	if len(parts) == 0 {
		parts = append(parts, "COUNT(*)")
	}

	b.WriteString("SELECT ")
	b.WriteString(strings.Join(parts, ", "))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(body.Table))

	whereClause, whereArgs, err := renderExpr(d, qualifyNode(body.Where, ""), counter)
	if err != nil {
		return nil, err
	}

	if whereClause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(whereClause)
	}

	args = append(args, whereArgs...)

	if len(body.GroupCols) > 0 {
		b.WriteString(" GROUP BY ")
		b.WriteString(joinQuoted(d, body.GroupCols))
	}

	havingClause, havingArgs, err := renderHaving(d, body.Having, counter)
	if err != nil {
		return nil, err
	}

	if havingClause != "" {
		b.WriteString(" HAVING ")
		b.WriteString(havingClause)
	}

	args = append(args, havingArgs...)

	return args, nil
}

// writeSelectTail renders the shared trailing clauses of a SELECT --
// [WHERE ...] [ORDER BY ...] [LIMIT ...] [OFFSET ...] -- into b,
// numbering placeholders with counter and returning the bound arguments.
// It is the exact clause sequence Select/SelectJoin/GroupedSelect/
// SelectWith each repeat, factored out so a CTE's body and outer SELECT
// share one counter across the whole statement. err is non-nil when where
// contains an unsupported NodeKind/Op.
func writeSelectTail(b *strings.Builder, d dialect.Dialect, where Node, order []OrderTerm, limit, offset int, counter *argCounter) ([]any, error) {
	var args []any

	clause, whereArgs, err := renderExpr(d, where, counter)
	if err != nil {
		return nil, err
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
			return nil, err
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

	return args, nil
}
