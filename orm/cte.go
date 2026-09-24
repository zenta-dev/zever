package orm

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// ErrCTEClauseRequiresRecursive is returned when a SEARCH or CYCLE clause is
// requested on a plain (non-recursive) CTE. Both clauses are defined only for
// a WITH RECURSIVE definition, so With(...).SearchDepthFirst(...) /
// .Cycle(...) is a caller error, rejected with a typed error before the
// dialect capability is even consulted. Callers test with errors.Is.
var ErrCTEClauseRequiresRecursive = errors.New("orm: SEARCH/CYCLE require a recursive CTE (use WithRecursive)")

// ErrCTEClauseEmptyColumns is returned when a SEARCH or CYCLE clause is
// requested with no BY columns. Both clauses require at least one column to
// order the traversal / identify a repeated row, so an empty list would
// render invalid SQL; it is rejected with a typed error instead. Callers
// test with errors.Is.
var ErrCTEClauseEmptyColumns = errors.New("orm: SEARCH/CYCLE require at least one BY column")

// CTEName is a validated common-table-expression identifier. Its field is
// unexported: the only way to construct one is NewCTEName, which validates
// the identifier strictly, so a CTE name is always a safe, quotable
// identifier -- never a bare caller string that could break out of the
// double-quoted identifier the renderer emits ("safe by
// default" rule; CTE names have no codegen source yet, so a validated,
// error-returning constructor is the narrowest available mitigation, to be
// replaced by codegen/constants in a later phase).
type CTEName struct{ name string }

// NewCTEName validates s as a CTE identifier and returns a CTEName, or an
// error if s is empty, starts with a digit, or contains any character
// outside [A-Za-z0-9_]. It is error-returning, never panicking.
func NewCTEName(s string) (CTEName, error) {
	if s == "" {
		return CTEName{}, errors.New("orm: CTE name must not be empty")
	}

	for i, r := range s {
		if !isCTENameChar(r) {
			return CTEName{}, fmt.Errorf("orm: invalid CTE name %q: character %q at position %d (only [A-Za-z0-9_] allowed)", s, r, i)
		}

		if i == 0 && r >= '0' && r <= '9' {
			return CTEName{}, fmt.Errorf("orm: invalid CTE name %q: must not start with a digit", s)
		}
	}

	return CTEName{name: s}, nil
}

// isCTENameChar reports whether r is allowed in a CTE identifier: the
// [A-Za-z0-9_] charset NewCTEName's validation enforces. With the charset
// pinned and every CTE name emitted through a double-quoting QuoteIdent,
// no caller-supplied string can break out of the identifier boundary.
func isCTENameChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// CTETable builds a Table reference bound to a CTE's output columns,
// for use inside a recursive CTE's recursive branch: the recursive branch
// joins the entity's own table back to the CTE being defined, so it needs a
// Table reference whose Name() is the CTE's name. Its column list is empty
// (a CTE reference is only ever used as a join target, never scanned
// directly -- the outer query over the CTE does the scanning).
func CTETable[T any](name CTEName) Table[T] {
	return Table[T]{name: name.name}
}

// requireCTE reports whether d can run the CTE statement. A plain WITH is
// gated by dialect.CTEDialect with SupportsCTE reporting true; WITH
// RECURSIVE additionally requires SupportsRecursive() to report true. A
// missing capability returns a typed dialect.ErrUnsupportedByDialect --
// never a panic or a silent wrong-SQL fallback. The
// SupportsCTE split exists because presence of the CTEDialect method set
// is not enough on its own: MySQL 5.7 implements the method set yet
// supports no CTEs at all (they arrive in 8.0), so a version-gated
// SupportsCTE() is what actually rejects it.
func requireCTE(d dialect.Dialect, recursive bool) error {
	cd, ok := d.(dialect.CTEDialect)
	if !ok || !cd.SupportsCTE() {
		return fmt.Errorf("orm: %w: dialect %q does not support common table expressions", dialect.ErrUnsupportedByDialect, d.Name())
	}

	if recursive && !cd.SupportsRecursive() {
		return fmt.Errorf("orm: %w: dialect %q does not support recursive CTEs", dialect.ErrUnsupportedByDialect, d.Name())
	}

	return nil
}

// requireCTEExtensions gates the two optional CTE extensions on d: the
// `[NOT] MATERIALIZED` modifier (dialect.CTEMaterializationDialect) and the
// recursive `SEARCH` / `CYCLE` clauses (dialect.CTESearchCycleDialect). A
// missing capability returns the typed dialect.ErrUnsupportedByDialect,
// never a silent wrong-SQL fallback. A SEARCH/CYCLE clause on a non-recursive
// body is a caller error (ErrCTEClauseRequiresRecursive), checked before the
// dialect gate so the message names the actual misuse.
func requireCTEExtensions(d dialect.Dialect, body render.CTEBody, recursive bool) error {
	if body.Materialization != render.CTEMaterializeDefault {
		md, ok := d.(dialect.CTEMaterializationDialect)
		if !ok || !md.SupportsCTEMaterialized() {
			return fmt.Errorf("orm: %w: dialect %q does not support the CTE MATERIALIZED modifier", dialect.ErrUnsupportedByDialect, d.Name())
		}
	}

	searchSet, cycleSet := body.SearchSet != "", body.CycleSet != ""
	if !searchSet && !cycleSet {
		return nil
	}

	if !recursive {
		return ErrCTEClauseRequiresRecursive
	}

	if (searchSet && len(body.SearchBy) == 0) || (cycleSet && len(body.CycleBy) == 0) {
		return ErrCTEClauseEmptyColumns
	}

	sc, ok := d.(dialect.CTESearchCycleDialect)
	if !ok || !sc.SupportsCTESearchCycle() {
		return fmt.Errorf("orm: %w: dialect %q does not support the recursive CTE SEARCH/CYCLE clauses", dialect.ErrUnsupportedByDialect, d.Name())
	}

	return nil
}

// combineNodes ANDs two erased predicate nodes together, skipping whichever
// side (or both) is unset -- mirroring orm.And's zero/one-element
// collapsing rule for the two-different-T case (Predicate[A] and
// Predicate[B]) where orm.And's single-T generic cannot apply. It operates
// on render.Node values because that is the shape the outer SELECT's WHERE
// clause is consumed in (see toRenderNode).
func combineNodes(a, b render.Node) render.Node {
	switch {
	case a.Kind == render.KindNone && b.Kind == render.KindNone:
		return render.Node{}
	case a.Kind == render.KindNone:
		return b
	case b.Kind == render.KindNone:
		return a
	default:
		return render.Node{Kind: render.KindCompound, Compound: render.CompoundAnd, Children: []render.Node{a, b}}
	}
}

// cteBody converts a plain Query into the erased CTE body shape for
// With/WithRecursive.
func (q Query[T, PT]) cteBody() render.CTEBody {
	return render.CTEBody{
		Kind:    render.CTEBodyPlain,
		Table:   q.table.Name(),
		Columns: q.table.Columns(),
		Where:   toRenderNode[T](q.where.Render()),
		Order:   toRenderOrder(q.order),
		Limit:   q.limit,
		Offset:  q.offset,
	}
}

// CTEQuery is an immutable, value-type builder over a CTE's result: it
// wraps a named Query[T, PT] (or a recursive CTE's base+recursive pair)
// into `WITH [RECURSIVE] "name" AS (<body>) SELECT ... FROM "name"`, and
// follows the same copy-on-write chain discipline as Query[T, PT] --
// Where/OrderBy/Limit/Offset each return a NEW value, and OrderBy copies
// into a fresh backing array first.
//
// The outer SELECT projects the wrapped query's own columns in the same
// order, so All/First scan rows through T's codegen'd Scan method exactly
// as if the CTE were the entity's own table. Where/OrderBy apply to the
// OUTER query, whose FROM is only the CTE -- the predicate/order columns
// are the entity's (unqualified) column names, which resolve against the
// CTE's output.
type CTEQuery[T any, PT ptrScanner[T]] struct {
	name      string
	recursive bool
	body      render.CTEBody
	outerCols []string
	where     Predicate[T]
	order     []OrderTerm[T]
	limit     int
	offset    int
}

// With wraps inner's SELECT into `WITH "name" AS (<inner>)` and yields a
// query over the CTE. The capability is checked at All/First/Count time --
// the dialect is resolved from exec there, the same timing every other
// orm query resolves it -- via dialect.CTEDialect (see requireCTE).
func With[T any, PT ptrScanner[T]](name CTEName, inner Query[T, PT]) CTEQuery[T, PT] {
	return CTEQuery[T, PT]{
		name:      name.name,
		body:      inner.cteBody(),
		outerCols: inner.table.Columns(),
	}
}

// WithRecursive wraps a recursive CTE: `WITH RECURSIVE "name" AS
// (<base> UNION ALL <recursive>)`, where
//
// - base is the anchor select (e.g. the tree's roots), carrying its own
// WHERE filter, and
// - recursive joins the entity's table back to the CTE itself via
// cteRel -- a Relation whose child side is a CTETable reference to the
// very CTE being defined (e.g. `NewRelation[T, T](childKey, parentKey,
// CTETable[T](name))`), additionally filtered by recursiveWhere.
//
// Requires dialect.CTEDialect with SupportsRecursive() reporting true (both
// SQLite and Postgres do); a dialect lacking it returns a typed
// dialect.ErrUnsupportedByDialect at All/First/Count time.
func WithRecursive[T any, PT ptrScanner[T]](name CTEName, base Query[T, PT], recursiveWhere Predicate[T], cteRel Relation[T, T]) CTEQuery[T, PT] {
	baseBody := base.cteBody()
	baseBody.Order = nil
	baseBody.Limit = 0
	baseBody.Offset = 0

	recursive := render.CTEBody{
		Kind:      render.CTEBodyPlain,
		Table:     base.table.Name(),
		Columns:   base.table.Columns(),
		Where:     toRenderNode[T](recursiveWhere.Render()),
		JoinTable: name.name,
		ParentCol: cteRel.parentCol,
		ChildCol:  cteRel.childCol,
	}

	return CTEQuery[T, PT]{
		name:      name.name,
		recursive: true,
		body: render.CTEBody{
			Kind:      render.CTEBodySetOp,
			SetOp:     render.SetOpUnionAll,
			LeftBody:  &baseBody,
			RightBody: &recursive,
		},
		outerCols: base.table.Columns(),
	}
}

// Where combines p into c's outer-query filter with AND -- the receiver is
// left unmodified, matching Query[T, PT].Where's rule exactly.
func (c CTEQuery[T, PT]) Where(p Predicate[T]) CTEQuery[T, PT] {
	if c.where.IsSet() {
		c.where = And(c.where, p)
	} else {
		c.where = p
	}

	return c
}

// OrderBy appends terms to the outer query's ORDER BY list. It copies
// c.order into a FRESH backing array before appending -- never a bare
// append(c.order, ...) -- matching Query[T, PT].OrderBy's branch-safety
// rule.
func (c CTEQuery[T, PT]) OrderBy(terms ...OrderTerm[T]) CTEQuery[T, PT] {
	next := make([]OrderTerm[T], 0, len(c.order)+len(terms))
	next = append(next, c.order...)
	next = append(next, terms...)
	c.order = next

	return c
}

// Limit sets the outer query's row limit.
func (c CTEQuery[T, PT]) Limit(n int) CTEQuery[T, PT] {
	c.limit = n

	return c
}

// Offset sets the outer query's row offset.
func (c CTEQuery[T, PT]) Offset(n int) CTEQuery[T, PT] {
	c.offset = n

	return c
}

// Materialized adds the `AS MATERIALIZED (...)` hint to the CTE definition,
// telling the planner to always materialize it (Postgres 12+/SQLite 3.35+).
// It requires dialect.CTEMaterializationDialect; MySQL and older
// SQLite/Postgres return a typed dialect.ErrUnsupportedByDialect at
// execution time. The receiver is left unmodified; the copy-on-write chain
// discipline matches Query[T, PT].
func (c CTEQuery[T, PT]) Materialized() CTEQuery[T, PT] {
	c.body.Materialization = render.CTEMaterializeAlways

	return c
}

// NotMaterialized adds the `AS NOT MATERIALIZED (...)` hint to the CTE
// definition, letting the planner inline it into the outer query instead of
// materializing it (Postgres 12+/SQLite 3.35+). It is gated exactly like
// Materialized; see that method.
func (c CTEQuery[T, PT]) NotMaterialized() CTEQuery[T, PT] {
	c.body.Materialization = render.CTEMaterializeNever

	return c
}

// SearchDepthFirst adds a `SEARCH DEPTH FIRST BY <by...> SET <orderCol>`
// clause to a recursive CTE: Postgres computes the depth-first ordering key
// and exposes it as a new column named orderCol (a validated identifier; see
// NewCTEName), so the OUTER query can ORDER BY it through a generated column
// of the same name. by lists the CTE's own output columns that define the
// traversal order. Only meaningful on WithRecursive -- calling it on a plain
// With returns ErrCTEClauseRequiresRecursive at execution time -- and gated
// by dialect.CTESearchCycleDialect, so SQLite/MySQL/older Postgres return a
// typed dialect.ErrUnsupportedByDialect.
func (c CTEQuery[T, PT]) SearchDepthFirst(orderCol CTEName, by ...AnyColumn[T]) CTEQuery[T, PT] {
	c.body.SearchSet = orderCol.name
	c.body.SearchBy = anyColumnNames(by)

	return c
}

// Cycle adds a `CYCLE <by...> SET <isCycleCol> USING <pathCol>` clause to a
// recursive CTE: Postgres stops recursing when a row repeats along the
// traversal path (breaking cycles in graph-shaped data), marking the row via
// the new boolean column isCycleCol and tracking the visited path in the new
// array column pathCol (both validated identifiers; see NewCTEName). This is
// the safe way to recurse over a cyclic graph -- without it a cyclic input
// does not terminate. Gated exactly like SearchDepthFirst.
func (c CTEQuery[T, PT]) Cycle(isCycleCol, pathCol CTEName, by ...AnyColumn[T]) CTEQuery[T, PT] {
	c.body.CycleSet = isCycleCol.name
	c.body.CycleUsing = pathCol.name
	c.body.CycleBy = anyColumnNames(by)

	return c
}

// requireExtensions gates c's optional CTE extensions against d, also
// rejecting a SEARCH/CYCLE clause on a non-recursive CTE. It is the single
// gate shared by All, Count, and the Explain paths.
func (c CTEQuery[T, PT]) requireExtensions(d dialect.Dialect) error {
	return requireCTEExtensions(d, c.body, c.recursive)
}

// All runs c against exec and returns every matching row, scanned via T's
// codegen'd Scan method with zero reflection -- the CTE's output columns
// match the wrapped query's own columns positionally.
func (c CTEQuery[T, PT]) All(ctx context.Context, exec db.DB) ([]PT, error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	err = requireCTE(d, c.recursive)
	if err != nil {
		return nil, err
	}

	err = c.requireExtensions(d)
	if err != nil {
		return nil, err
	}

	query, args, err := render.SelectWith(d, c.name, c.recursive, c.body, c.outerCols, toRenderNode[T](c.where.Render()), toRenderOrder(c.order), c.limit, c.offset)
	if err != nil {
		return nil, fmt.Errorf("orm: CTEQuery.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: CTEQuery.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []PT

	for rows.Next() {
		var v T

		p := PT(&v)
		if err := p.Scan(rows); err != nil {
			return nil, fmt.Errorf("orm: CTEQuery.All: scan: %w", err)
		}

		out = append(out, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: CTEQuery.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: CTEQuery.All: %w", err)
	}

	return out, nil
}

// First runs c with an implicit LIMIT 1 and returns the first matching row,
// or ok=false if there was none.
func (c CTEQuery[T, PT]) First(ctx context.Context, exec db.DB) (row PT, ok bool, err error) {
	rows, err := c.Limit(1).All(ctx, exec)
	if err != nil {
		return nil, false, err
	}

	if len(rows) == 0 {
		return nil, false, nil
	}

	return rows[0], true, nil
}

// Count runs `WITH ... AS (<body>) SELECT COUNT(*) FROM "name"` for the
// CTE, ignoring the outer query's order/limit/offset (meaningless for a
// count).
func (c CTEQuery[T, PT]) Count(ctx context.Context, exec db.DB) (int64, error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return 0, err
	}

	err = requireCTE(d, c.recursive)
	if err != nil {
		return 0, err
	}

	err = c.requireExtensions(d)
	if err != nil {
		return 0, err
	}

	query, args, err := render.CountWith(d, c.name, c.recursive, c.body)
	if err != nil {
		return 0, fmt.Errorf("orm: CTEQuery.Count: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return 0, fmt.Errorf("orm: CTEQuery.Count: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var n int64

	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, fmt.Errorf("orm: CTEQuery.Count: scan: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("orm: CTEQuery.Count: %w", err)
	}

	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("orm: CTEQuery.Count: %w", err)
	}

	return n, nil
}

// cteBody converts a Join2 into the erased CTE body shape for WithJoin.
func (j Join2[A, PA, B, PB]) cteBody() render.CTEBody {
	return render.CTEBody{
		Kind:         render.CTEBodyJoin,
		Table:        j.left.table.Name(),
		Columns:      j.left.table.Columns(),
		Where:        toRenderNode[A](j.left.where.Render()),
		Order:        toRenderOrder(j.left.order),
		Limit:        j.left.limit,
		Offset:       j.left.offset,
		JoinType:     j.joinType,
		RightTable:   j.rel.childTable.Name(),
		RightColumns: j.rel.childTable.Columns(),
		ParentCol:    j.rel.parentCol,
		ChildCol:     j.rel.childCol,
		WhereRight:   toRenderNode[B](j.whereRight.Render()),
		OrderRight:   toRenderOrder(j.orderRight),
	}
}

// CTEJoin is the CTE wrapper for a Join2: it wraps the join's projecting
// SELECT into `WITH "name" AS (<join>)` and yields a query over the CTE
// whose All returns Row2[A, B] via the same single-Scan-call
// scanJoinRow plumbing Join2 itself uses.
//
// The CTE's output columns are the left table's columns followed by the
// right table's, so Where/WhereRight predicates -- which reference the
// entity columns by their bare names -- resolve against the CTE's output.
// A column name shared by both tables makes an unqualified outer predicate
// on that name ambiguous; prefer joining tables with disjoint column sets,
// or filter client-side.
type CTEJoin[A any, PA ptrScanner[A], B any, PB ptrScanner[B]] struct {
	name       string
	body       render.CTEBody
	outerCols  []string
	aCols      int
	bCols      int
	whereLeft  Predicate[A]
	whereRight Predicate[B]
	order      []OrderTerm[A]
	orderRight []OrderTerm[B]
	limit      int
	offset     int
}

// WithJoin wraps inner's join SELECT into a named CTE. The capability is
// checked at All time via dialect.CTEDialect (see requireCTE).
func WithJoin[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](name CTEName, inner Join2[A, PA, B, PB]) CTEJoin[A, PA, B, PB] {
	leftCols := inner.left.table.Columns()
	rightCols := inner.rel.childTable.Columns()

	outerCols := make([]string, 0, len(leftCols)+len(rightCols))
	outerCols = append(outerCols, leftCols...)
	outerCols = append(outerCols, rightCols...)

	return CTEJoin[A, PA, B, PB]{
		name:      name.name,
		body:      inner.cteBody(),
		outerCols: outerCols,
		aCols:     len(leftCols),
		bCols:     len(rightCols),
	}
}

// Where combines p into the outer query's left-table (A) filter with AND,
// matching Join2.Where's copy-on-write rule.
func (j CTEJoin[A, PA, B, PB]) Where(p Predicate[A]) CTEJoin[A, PA, B, PB] {
	if j.whereLeft.IsSet() {
		j.whereLeft = And(j.whereLeft, p)
	} else {
		j.whereLeft = p
	}

	return j
}

// WhereRight combines p into the outer query's right-table (B) filter with
// AND; see CTEJoin's doc comment for the shared-column-name caveat.
func (j CTEJoin[A, PA, B, PB]) WhereRight(p Predicate[B]) CTEJoin[A, PA, B, PB] {
	if j.whereRight.IsSet() {
		j.whereRight = And(j.whereRight, p)
	} else {
		j.whereRight = p
	}

	return j
}

// OrderBy appends terms to the outer query's ORDER BY list, scoped to the
// left table (its columns are the CTE's leading output columns). It copies
// j.order into a FRESH backing array before appending.
func (j CTEJoin[A, PA, B, PB]) OrderBy(terms ...OrderTerm[A]) CTEJoin[A, PA, B, PB] {
	next := make([]OrderTerm[A], 0, len(j.order)+len(terms))
	next = append(next, j.order...)
	next = append(next, terms...)
	j.order = next

	return j
}

// OrderByRight appends terms to the outer query's ORDER BY list, scoped to
// the right table (its columns are the CTE's trailing output columns after
// the left table's). It copies j.orderRight into a FRESH backing array
// before appending.
func (j CTEJoin[A, PA, B, PB]) OrderByRight(terms ...OrderTerm[B]) CTEJoin[A, PA, B, PB] {
	next := make([]OrderTerm[B], 0, len(j.orderRight)+len(terms))
	next = append(next, j.orderRight...)
	next = append(next, terms...)
	j.orderRight = next

	return j
}

// Limit sets the outer query's row limit.
func (j CTEJoin[A, PA, B, PB]) Limit(n int) CTEJoin[A, PA, B, PB] {
	j.limit = n

	return j
}

// Materialized adds the `AS MATERIALIZED (...)` hint to the wrapped join CTE;
// see CTEQuery.Materialized for the capability gate.
func (j CTEJoin[A, PA, B, PB]) Materialized() CTEJoin[A, PA, B, PB] {
	j.body.Materialization = render.CTEMaterializeAlways

	return j
}

// NotMaterialized adds the `AS NOT MATERIALIZED (...)` hint to the wrapped
// join CTE; see CTEQuery.Materialized for the capability gate.
func (j CTEJoin[A, PA, B, PB]) NotMaterialized() CTEJoin[A, PA, B, PB] {
	j.body.Materialization = render.CTEMaterializeNever

	return j
}

// renderOuter renders the whole `WITH "name" AS (<join body>) SELECT ...`
// statement, gating both the CTE capability (requireCTE) and the join
// body's own join keyword (requireJoin, so a RIGHT/FULL join body a
// dialect would reject never reaches the server). It is the single
// execution path shared by All and Explain.
func (j CTEJoin[A, PA, B, PB]) renderOuter(d dialect.Dialect) (string, []any, error) {
	if err := requireCTE(d, false); err != nil {
		return "", nil, err
	}

	if err := requireCTEExtensions(d, j.body, false); err != nil {
		return "", nil, err
	}

	if err := requireJoin(d, j.body.JoinType); err != nil {
		return "", nil, err
	}

	outerWhere := combineNodes(toRenderNode[A](j.whereLeft.Render()), toRenderNode[B](j.whereRight.Render()))

	outerOrder := append(toRenderOrder(j.order), toRenderOrder(j.orderRight)...)

	return render.SelectWith(d, j.name, false, j.body, j.outerCols, outerWhere, outerOrder, j.limit, j.offset)
}

// All runs j against exec and returns every matching row as a typed
// Row2[A, B], scanned via ONE rows.Scan call per row spanning both A's and
// B's columns (see scanJoinRow).
func (j CTEJoin[A, PA, B, PB]) All(ctx context.Context, exec db.DB) ([]Row2[A, B], error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	query, args, err := j.renderOuter(d)
	if err != nil {
		return nil, fmt.Errorf("orm: CTEJoin.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: CTEJoin.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Row2[A, B]

	for rows.Next() {
		row, err := scanJoinRow[A, PA, B, PB](rows, j.aCols, j.bCols)
		if err != nil {
			return nil, fmt.Errorf("orm: CTEJoin.All: scan: %w", err)
		}

		out = append(out, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: CTEJoin.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: CTEJoin.All: %w", err)
	}

	return out, nil
}

// cteBody converts a GroupedQuery into the erased CTE body shape for
// WithGrouped.
func (g GroupedQuery[T]) cteBody() render.CTEBody {
	groupCols := make([]string, len(g.groupCols))
	for i, c := range g.groupCols {
		groupCols[i] = c.Name()
	}

	return render.CTEBody{
		Kind:      render.CTEBodyGrouped,
		Table:     g.table.Name(),
		Where:     toRenderNode[T](g.where.Render()),
		GroupCols: groupCols,
		Aggs:      toRenderAggregates(g.aggs),
		Having:    toRenderHaving(g.having.n),
	}
}

// CTEGroupedQuery is the CTE wrapper for a GroupedQuery: it wraps the
// grouped aggregate SELECT into `WITH "name" AS (<grouped>)` and yields a
// query over the CTE whose Scan hands each result row to a callback, the
// same shape GroupedQuery.Scan itself uses (an aggregate result row does
// not correspond to any single T, so a callback over the raw Row is the
// only scan shape that doesn't require generating a bespoke result struct).
// Result columns are the group columns in GroupBy order, then each
// aggregate in Agg order -- the CTE's own output columns, read positionally.
type CTEGroupedQuery[T any] struct {
	name      string
	body      render.CTEBody
	outerCols []string
	order     []OrderTerm[T]
	// advancedGroups records that the wrapped GroupedQuery used a GROUP BY
	// expression or grouping construct, which the CTE's by-name outer
	// projection cannot express; Scan/Explain surface a typed error rather
	// than silently dropping it.
	advancedGroups bool
}

// WithGrouped wraps inner's grouped aggregate SELECT into a named CTE. The
// capability is checked at Scan time via dialect.CTEDialect (see
// requireCTE).
func WithGrouped[T any](name CTEName, inner GroupedQuery[T]) CTEGroupedQuery[T] {
	outerCols := make([]string, 0, len(inner.groupCols)+len(inner.aggs))

	for _, c := range inner.groupCols {
		outerCols = append(outerCols, c.Name())
	}

	for _, a := range inner.aggs {
		outerCols = append(outerCols, a.Alias)
	}

	return CTEGroupedQuery[T]{
		name:           name.name,
		body:           inner.cteBody(),
		outerCols:      outerCols,
		advancedGroups: inner.hasAdvancedGroups(),
	}
}

// OrderBy appends terms to the outer query's ORDER BY list. Terms should
// reference group-column names (bare) -- the CTE's leading output columns.
func (g CTEGroupedQuery[T]) OrderBy(terms ...OrderTerm[T]) CTEGroupedQuery[T] {
	next := make([]OrderTerm[T], 0, len(g.order)+len(terms))
	next = append(next, g.order...)
	next = append(next, terms...)
	g.order = next

	return g
}

// Materialized adds the `AS MATERIALIZED (...)` hint to the wrapped grouped
// CTE; see CTEQuery.Materialized for the capability gate.
func (g CTEGroupedQuery[T]) Materialized() CTEGroupedQuery[T] {
	g.body.Materialization = render.CTEMaterializeAlways

	return g
}

// NotMaterialized adds the `AS NOT MATERIALIZED (...)` hint to the wrapped
// grouped CTE; see CTEQuery.Materialized for the capability gate.
func (g CTEGroupedQuery[T]) NotMaterialized() CTEGroupedQuery[T] {
	g.body.Materialization = render.CTEMaterializeNever

	return g
}

// Scan runs g against exec and hands every result row to fn. Result
// columns are the wrapped query's group columns in order, then each
// aggregate in the order Agg was called. It returns an error if any
// aggregate in the wrapped GroupedQuery carries an empty Alias: the CTE's
// outer SELECT projects the aggregates BY their aliases, so a missing
// alias would render `SELECT "" FROM ...` rather than a named column.
func (g CTEGroupedQuery[T]) Scan(ctx context.Context, exec db.DB, fn func(row Row) error) error {
	if err := g.requireSimpleGroups(); err != nil {
		return err
	}

	if err := g.requireAggAliases(); err != nil {
		return err
	}

	d, err := resolveDialect(exec)
	if err != nil {
		return err
	}

	err = requireCTE(d, false)
	if err != nil {
		return err
	}

	err = requireCTEExtensions(d, g.body, false)
	if err != nil {
		return err
	}

	query, args, err := render.SelectWith(d, g.name, false, g.body, g.outerCols, render.Node{}, toRenderOrder(g.order), 0, 0)
	if err != nil {
		return fmt.Errorf("orm: CTEGroupedQuery.Scan: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return fmt.Errorf("orm: CTEGroupedQuery.Scan: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		if err := fn(rows); err != nil {
			_ = rows.Close()

			return fmt.Errorf("orm: CTEGroupedQuery.Scan: row: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: CTEGroupedQuery.Scan: %w", err)
	}

	if err := rows.Close(); err != nil {
		return fmt.Errorf("orm: CTEGroupedQuery.Scan: %w", err)
	}

	return nil
}

// requireSimpleGroups rejects a grouped CTE whose wrapped GroupedQuery used
// an expression or grouping construct in its GROUP BY. The CTE's outer SELECT projects its
// group columns by name (see WithGrouped), so such a grouping cannot be
// expressed -- surfaced as a typed dialect.ErrUnsupportedByDialect rather
// than silently dropped.
func (g CTEGroupedQuery[T]) requireSimpleGroups() error {
	if !g.advancedGroups {
		return nil
	}

	return fmt.Errorf(
		"orm: CTEGroupedQuery: %w: a CTE projects group columns by name and cannot express an expression or grouping-construct GROUP BY",
		dialect.ErrUnsupportedByDialect,
	)
}

// requireAggAliases rejects a grouped CTE whose body carries an aggregate
// without an alias. The outer SELECT projects `FROM "cte"` selecting the
// aliases by name (see WithGrouped), so an empty alias would silently
// render `SELECT "" FROM ...` -- a caller bug, surfaced as an error rather
// than broken SQL.
func (g CTEGroupedQuery[T]) requireAggAliases() error {
	for _, a := range g.body.Aggs {
		if a.Alias == "" {
			return fmt.Errorf("orm: aggregate %s requires an alias (WithGrouped projects the CTE's aggregate columns by alias)", a.Column)
		}
	}

	return nil
}
