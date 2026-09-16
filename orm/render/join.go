package render

import (
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// JoinType mirrors the builder's JoinType values -- the same "render depends only
// on its own copy of the enum, never imports the orm root" rule Op/NodeKind/
// CompoundOp above already follow, so the join builder converts with a plain Go
// type conversion (render.JoinType(j.joinType)).
type JoinType int

// Supported join types. INNER and LEFT are universal SQL; RIGHT and FULL
// are gated per-dialect (see dialect.JoinCapabilities) -- render emits
// the keyword a caller asked for, and the gate lives at the caller's
// execution boundary, never silently substituting a weaker join.
const (
	InnerJoin JoinType = iota
	LeftJoin
	RightJoin
	FullJoin
)

func (jt JoinType) keyword() string {
	switch jt {
	case InnerJoin:
		return "INNER JOIN"
	case LeftJoin:
		return "LEFT JOIN"
	case RightJoin:
		return "RIGHT JOIN"
	case FullJoin:
		return "FULL JOIN"
	default:
		return "INNER JOIN"
	}
}

// SelectJoin renders a single projecting two-table SELECT: an explicit
// qualified column list from BOTH tables (never `SELECT *`), one JOIN
// clause (INNER/LEFT/RIGHT/FULL per joinType), and a WHERE clause combining
// whereLeft (qualified to leftTable) and whereRight (qualified to
// rightTable) with AND. This is the "projecting" variant the join builder's
// Join2/LeftJoin2/RightJoin2/FullJoin2 need -- unlike orm/engine's
// writeJoins (see orm/engine/join.go), which only ever projects the LEFT
// table's columns and treats a join purely as a WHERE-clause constraint,
// this renders BOTH sides' columns so a single rows.Scan call can populate
// both halves of a Row2[A, B].
//
// whereLeft/whereRight's Node values are expected to carry their own real
// table name in every leaf's Table field (true for every Node the Column/
// NullableColumn construct, since the column constructors are
// always given the entity's real SQL table name) -- qualifyNode uses that
// to prefix each column reference, so callers never need to pass a
// separate "default table" fallback.
//
// orderLeft is qualified to leftTable and orderRight to rightTable, so a
// caller can order by either table's columns independently -- the ORDER BY
// analog of the Where/WhereRight split (see the join builder's OrderByRight).
func SelectJoin(
	d dialect.Dialect,
	joinType JoinType,
	leftTable string, leftColumns []string,
	rightTable string, rightColumns []string,
	parentCol, childCol string,
	whereLeft, whereRight Node,
	orderLeft, orderRight []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	var b strings.Builder

	b.WriteString("SELECT ")
	b.WriteString(joinQualifiedColumns(d, []string{leftTable, rightTable}, [][]string{leftColumns, rightColumns}))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(leftTable))
	b.WriteString(" ")
	b.WriteString(joinType.keyword())
	b.WriteString(" ")
	b.WriteString(d.QuoteIdent(rightTable))
	b.WriteString(" ON ")
	b.WriteString(quoteColumn(d, leftTable+"."+parentCol))
	b.WriteString(" = ")
	b.WriteString(quoteColumn(d, rightTable+"."+childCol))

	counter := &argCounter{}

	combined := combineWhere(qualifyNode(whereLeft, ""), qualifyNode(whereRight, ""))

	// The join is itself a multi-table enclosing statement: a subquery
	// nested in its WHERE correlates against BOTH of its tables, so a
	// marker may reference either side. The join's own WHERE has no
	// enclosing statement, so a bare marker there still fails closed.
	clause, whereArgs, err := renderExprCorr(d, combined, counter, renderScope{stmt: []string{leftTable, rightTable}})
	if err != nil {
		return "", nil, err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	args = append(args, whereArgs...)

	if len(orderLeft) > 0 || len(orderRight) > 0 {
		b.WriteString(" ORDER BY ")

		orderText, orderArgs, err := joinOrderByAll(d, []string{leftTable, rightTable}, [][]OrderTerm{orderLeft, orderRight}, counter)
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

// SelectJoin3 renders a single projecting three-table SELECT -- the Join3
// shape: an explicit qualified column list from ALL THREE tables, a chain
// of TWO JOIN clauses (both of joinType), and a WHERE clause combining
// whereA/whereB/whereC (each qualified to its own table) with AND. It is
// SelectJoin generalized to three tables, sharing the same qualified-column
// and per-table ORDER BY machinery so a single rows.Scan call can populate
// all three halves of a Row3[A, B, C].
//
// It delegates to selectJoin3 with the same keyword on both hops, so the
// homogeneous (#409) and mixed entry points below can never drift apart.
func SelectJoin3(
	d dialect.Dialect,
	joinType JoinType,
	aTable string, aColumns []string,
	bTable string, bColumns []string,
	cTable string, cColumns []string,
	relABParent, relABChild, relBCParent, relBCChild string,
	whereA, whereB, whereC Node,
	orderA, orderB, orderC []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	return selectJoin3(
		d, joinType, joinType, false,
		aTable, aColumns, bTable, bColumns, cTable, cColumns,
		relABParent, relABChild, relBCParent, relBCChild,
		whereA, whereB, whereC, orderA, orderB, orderC, limit, offset,
	)
}

// SelectJoin3Mixed renders a three-table SELECT whose two JOIN clauses may
// carry DIFFERENT keywords -- the mixed-chain case, e.g. "A INNER JOIN B ON
// ... LEFT JOIN C ON ...". The clauses render flat, in order, exactly as a
// caller writes them; joinAB is the A/B keyword and joinBC the B/C keyword.
//
// A flat mixed chain is only the right shape when the SECOND hop cannot
// remove rows the FIRST hop preserved. "INNER then LEFT" qualifies (the
// inner join already fixed the A/B set, and the trailing LEFT only adds C
// nullability). A chain whose second hop is a non-preserving join on B (for
// example "LEFT then INNER") must instead use SelectJoin3Nested, or SQL's
// left-associative parsing would silently drop the A rows the first LEFT
// join was asked to preserve.
func SelectJoin3Mixed(
	d dialect.Dialect,
	joinAB, joinBC JoinType,
	aTable string, aColumns []string,
	bTable string, bColumns []string,
	cTable string, cColumns []string,
	relABParent, relABChild, relBCParent, relBCChild string,
	whereA, whereB, whereC Node,
	orderA, orderB, orderC []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	return selectJoin3(
		d, joinAB, joinBC, false,
		aTable, aColumns, bTable, bColumns, cTable, cColumns,
		relABParent, relABChild, relBCParent, relBCChild,
		whereA, whereB, whereC, orderA, orderB, orderC, limit, offset,
	)
}

// SelectJoin3Nested renders a three-table SELECT whose B/C pair is joined
// FIRST and parenthesized as a single composite table, which is then joined
// to A: "A <joinAB> (B <joinBC> C ON ...) ON ...". joinBC therefore binds
// tighter than joinAB, unlike SQL's flat, left-associative chain.
//
// This is the faithful render for "LEFT then INNER" (A LEFT JOIN (B INNER
// JOIN C)): the composite is all-or-nothing, so A is always present and the
// B/C pair is optional as a unit (B Some <=> C Some). Rendering it flat
// would evaluate as (A LEFT JOIN B) INNER JOIN C and drop every A row whose
// B is absent, breaking the outer join's central promise.
func SelectJoin3Nested(
	d dialect.Dialect,
	joinAB, joinBC JoinType,
	aTable string, aColumns []string,
	bTable string, bColumns []string,
	cTable string, cColumns []string,
	relABParent, relABChild, relBCParent, relBCChild string,
	whereA, whereB, whereC Node,
	orderA, orderB, orderC []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	return selectJoin3(
		d, joinAB, joinBC, true,
		aTable, aColumns, bTable, bColumns, cTable, cColumns,
		relABParent, relABChild, relBCParent, relBCChild,
		whereA, whereB, whereC, orderA, orderB, orderC, limit, offset,
	)
}

// selectJoin3 is the single rendering body behind SelectJoin3,
// SelectJoin3Mixed and SelectJoin3Nested: joinAB/joinBC are the two hop
// keywords, and nestedBC selects between the flat chain
// ("A j1 B ON ... j2 C ON ...") and the parenthesized composite
// ("A j1 (B j2 C ON ...) ON ..."). Everything else -- the qualified column
// projection, WHERE combination, per-table ORDER BY, LIMIT and OFFSET -- is
// shared verbatim, so no variant can drift from the others.
func selectJoin3(
	d dialect.Dialect,
	joinAB, joinBC JoinType,
	nestedBC bool,
	aTable string, aColumns []string,
	bTable string, bColumns []string,
	cTable string, cColumns []string,
	relABParent, relABChild, relBCParent, relBCChild string,
	whereA, whereB, whereC Node,
	orderA, orderB, orderC []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	var b strings.Builder

	b.WriteString("SELECT ")
	b.WriteString(joinQualifiedColumns(d, []string{aTable, bTable, cTable}, [][]string{aColumns, bColumns, cColumns}))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(aTable))
	b.WriteString(" ")
	b.WriteString(joinAB.keyword())
	b.WriteString(" ")

	if nestedBC {
		b.WriteString("(")
		b.WriteString(d.QuoteIdent(bTable))
		b.WriteString(" ")
		b.WriteString(joinBC.keyword())
		b.WriteString(" ")
		b.WriteString(d.QuoteIdent(cTable))
		b.WriteString(" ON ")
		b.WriteString(quoteColumn(d, bTable+"."+relBCParent))
		b.WriteString(" = ")
		b.WriteString(quoteColumn(d, cTable+"."+relBCChild))
		b.WriteString(")")
		b.WriteString(" ON ")
		b.WriteString(quoteColumn(d, aTable+"."+relABParent))
		b.WriteString(" = ")
		b.WriteString(quoteColumn(d, bTable+"."+relABChild))
	} else {
		b.WriteString(d.QuoteIdent(bTable))
		b.WriteString(" ON ")
		b.WriteString(quoteColumn(d, aTable+"."+relABParent))
		b.WriteString(" = ")
		b.WriteString(quoteColumn(d, bTable+"."+relABChild))
		b.WriteString(" ")
		b.WriteString(joinBC.keyword())
		b.WriteString(" ")
		b.WriteString(d.QuoteIdent(cTable))
		b.WriteString(" ON ")
		b.WriteString(quoteColumn(d, bTable+"."+relBCParent))
		b.WriteString(" = ")
		b.WriteString(quoteColumn(d, cTable+"."+relBCChild))
	}

	counter := &argCounter{}

	combined := combineWhere(combineWhere(qualifyNode(whereA, ""), qualifyNode(whereB, "")), qualifyNode(whereC, ""))

	// A subquery nested anywhere in the three-table join's WHERE correlates
	// against the join's full table set (A, B and C).
	clause, whereArgs, err := renderExprCorr(d, combined, counter, renderScope{stmt: []string{aTable, bTable, cTable}})
	if err != nil {
		return "", nil, err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	args = append(args, whereArgs...)

	if len(orderA) > 0 || len(orderB) > 0 || len(orderC) > 0 {
		b.WriteString(" ORDER BY ")

		orderText, orderArgs, err := joinOrderByAll(d, []string{aTable, bTable, cTable}, [][]OrderTerm{orderA, orderB, orderC}, counter)
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

// joinQualifiedColumns renders "t1.c1, t1.c2, ..., t2.c1, ..." -- each
// table's column list in order, qualified to its own table, matching the
// order the join builder's row scanners build their combined Scan
// destination list in.
func joinQualifiedColumns(d dialect.Dialect, tables []string, columns [][]string) string {
	total := 0

	for _, cols := range columns {
		total += len(cols)
	}

	all := make([]string, 0, total)

	for i, table := range tables {
		for _, c := range columns[i] {
			all = append(all, quoteColumn(d, table+"."+c))
		}
	}

	return strings.Join(all, ", ")
}

// qualifyNode returns a copy of n (and its Children, recursively) with
// every leaf's Column rewritten to "<table>.<Column>", where table is the
// leaf's own n.Table (which already carries the real SQL table name for
// every Column/NullableColumn-built leaf) or fallbackTable when n.Table is
// empty. A Column already carrying a "." (defensive; never produced by the
// public API today) is left alone.
func qualifyNode(n Node, fallbackTable string) Node {
	if n.Kind == KindNone {
		return n
	}

	if n.Column != "" && !strings.Contains(n.Column, ".") {
		table := n.Table
		if table == "" {
			table = fallbackTable
		}

		if table != "" {
			n.Column = table + "." + n.Column
		}
	}

	if len(n.Children) > 0 {
		children := make([]Node, len(n.Children))
		for i, c := range n.Children {
			children[i] = qualifyNode(c, fallbackTable)
		}

		n.Children = children
	}

	return n
}

// combineWhere ANDs two already-qualified where expressions together,
// skipping whichever side (or both) is unset -- mirroring
// the And combinator's zero/one-element collapsing rule.
func combineWhere(left, right Node) Node {
	switch {
	case left.Kind == KindNone && right.Kind == KindNone:
		return Node{}
	case left.Kind == KindNone:
		return right
	case right.Kind == KindNone:
		return left
	default:
		return Node{Kind: KindCompound, Compound: CompoundAnd, Children: []Node{left, right}}
	}
}

// qualifyOrderTerms prefixes every term's Column with table, unless it
// already carries a qualifier. An FTS ranking term keeps its FTS expression
// (whose column reference is the term's own Column, so the qualification
// applies to it too); a scalar-expression term is carried through unchanged
// (qualification applies to the expression's argument columns at build
// time, not here).
func qualifyOrderTerms(order []OrderTerm, table string) []OrderTerm {
	out := make([]OrderTerm, len(order))

	for i, o := range order {
		col := o.Column
		if !strings.Contains(col, ".") {
			col = table + "." + col
		}

		tbl := o.Table
		if tbl == "" {
			tbl = table
		}

		out[i] = OrderTerm{Column: col, Table: tbl, Desc: o.Desc, Nulls: o.Nulls, FTS: o.FTS, Func: o.Func}
	}

	return out
}

// joinOrderByAll renders one ORDER BY clause over N per-table term lists,
// each list qualified to its own table and the lists emitted in table order
// -- the shared machinery behind SelectJoin's and SelectJoin3's (and the
// CTE join body's) right-table-aware ordering. An empty list is skipped, so
// "order by B only" and "order by A then B" both render correctly. err is
// non-nil when a term's scalar expression is malformed (see joinOrderBy).
func joinOrderByAll(d dialect.Dialect, tables []string, orders [][]OrderTerm, counter *argCounter) (string, []any, error) {
	var (
		parts []string
		args  []any
	)

	for i, table := range tables {
		terms := orders[i]
		if len(terms) == 0 {
			continue
		}

		text, termArgs, err := joinOrderBy(d, qualifyOrderTerms(terms, table), counter)
		if err != nil {
			return "", nil, err
		}

		parts = append(parts, text)
		args = append(args, termArgs...)
	}

	return strings.Join(parts, ", "), args, nil
}

// SelectLateral renders a single projecting LATERAL join: an explicit
// qualified column list from BOTH the left table and the lateral subquery
// (never `SELECT *`), the `CROSS JOIN LATERAL (SELECT ...)` or
// `LEFT JOIN LATERAL (SELECT ...) ON TRUE` clause, an optional WHERE over
// the left table, an optional per-side ORDER BY, and LIMIT/OFFSET. It is
// the render behind the LateralJoin2/LeftLateralJoin2 builders.
//
// inner is the erased inner SELECT (see Subquery) and innerAlias the
// derived-table alias its projected columns are exposed under. The inner
// SELECT renders through renderSubquery with a scope whose corr is
// leftTable, so a the Outer/OuterNullable helpers marker in the inner WHERE resolves
// against the LEFT table at execution time -- exactly the correlated
// reference that makes a top-N-per-group lateral join work. The inner
// subquery's own ORDER BY/LIMIT/OFFSET render INSIDE the parentheses, and
// its bound arguments are numbered (and returned) BEFORE the outer WHERE's:
// the join clause precedes WHERE in the emitted SQL text, so that is the
// only ordering that keeps positional `?` dialects correct.
//
// cross selects the keyword pair: true renders `CROSS JOIN LATERAL (...)`
// (no ON condition -- a cross join has none), false renders
// `LEFT JOIN LATERAL (...) ON TRUE` (the mandatory ON clause, satisfied by
// the constant TRUE, so an outer row with no inner match is preserved with
// the inner columns all NULL). Left-join nullability is handled by the
// caller's scan path (scanLeftJoinRow), never here.
//
// orderLeft is qualified to leftTable and orderInner to innerAlias, so a
// caller can order by either side independently -- the ORDER BY analog of
// the outer WHERE / inner WHERE split.
func SelectLateral(
	d dialect.Dialect,
	cross bool,
	leftTable string, leftColumns []string,
	inner Subquery,
	innerAlias string,
	whereLeft Node,
	orderLeft, orderInner []OrderTerm,
	limit, offset int,
) (query string, args []any, err error) {
	var b strings.Builder

	b.WriteString("SELECT ")
	b.WriteString(joinQualifiedColumns(d, []string{leftTable, innerAlias}, [][]string{leftColumns, inner.Columns}))
	b.WriteString(" FROM ")
	b.WriteString(d.QuoteIdent(leftTable))
	b.WriteString(" ")

	if cross {
		b.WriteString("CROSS JOIN LATERAL ")
	} else {
		b.WriteString("LEFT JOIN LATERAL ")
	}

	b.WriteString("(")

	counter := &argCounter{}

	// The lateral subquery correlates against the LEFT table: its own scope
	// names its FROM table as stmt (so a marker inside IT binds one level
	// further in) and leftTable as corr (the enclosing table a marker in its
	// WHERE resolves against). renderSubquery derives exactly that from a
	// scope whose stmt is leftTable.
	innerSQL, innerArgs, err := renderSubquery(d, inner, counter, renderScope{stmt: []string{leftTable}})
	if err != nil {
		return "", nil, err
	}

	b.WriteString(innerSQL)
	b.WriteString(") AS ")
	b.WriteString(d.QuoteIdent(innerAlias))

	if !cross {
		b.WriteString(" ON TRUE")
	}

	args = append(args, innerArgs...)

	// The outer WHERE is the outermost statement's own filter: it has no
	// enclosing query, and any subquery nested in it correlates against
	// leftTable (the statement's own FROM table).
	clause, whereArgs, err := renderExprCorr(d, qualifyNode(whereLeft, ""), counter, renderScope{stmt: []string{leftTable}})
	if err != nil {
		return "", nil, err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	args = append(args, whereArgs...)

	if len(orderLeft) > 0 || len(orderInner) > 0 {
		b.WriteString(" ORDER BY ")

		orderText, orderArgs, err := joinOrderByAll(d, []string{leftTable, innerAlias}, [][]OrderTerm{orderLeft, orderInner}, counter)
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
