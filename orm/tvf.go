package orm

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// TableValuedSource is the FROM-source abstraction that lets a table-valued
// JSON function act as a join source with typed results. Query/Join2 are
// typed over a codegen Table[T] (a real SQL table); a TVF is not a table, so
// this interface carries the missing pieces: the derived-table alias every
// column reference is qualified with, the source's output column list in the
// order the row type B's Scan reads them, and a dialect-gated renderer for
// the function-call text.
//
// A concrete source is built by the dialect-specific JSON package that owns
// the function family -- orm/json/sqlite (json_each/json_tree),
// orm/json/postgres (jsonb_array_elements/...) --
// and usually also exposes typed Column[B, V] handles for its output columns,
// so WHERE/ORDER BY over the source compose with the existing
// predicate/ordering machinery (the handles carry the alias as their "table").
//
// B is the row type the source's columns scan into. It must implement
// Scanner (a Scan(Row) error method), exactly like a codegen entity, and its
// Scan must read exactly SrcColumns() in order. RenderSource performs the
// dialect capability gate and returns a typed dialect.ErrUnsupportedByDialect
// on a dialect that lacks the function family, never invalid SQL.
type TableValuedSource[B any] interface {
	// SrcAlias returns the derived-table alias the source is joined under.
	SrcAlias() string
	// SrcColumns returns the source's output columns, in Scan order.
	SrcColumns() []string
	// RenderSource renders the table-valued function call text (WITHOUT the
	// trailing ` AS <alias>`), gated on the dialect. It binds no arguments.
	RenderSource(d dialect.Dialect) (string, error)
	// NewRow returns a fresh, zero B ready to be scanned. It is the
	// structural tie that lets Go infer B from a concrete source, and it lets
	// a source return a non-zero default row when one is meaningful.
	NewRow() B
}

// TVFJoin2 is an immutable, value-type CROSS-JOIN builder whose right side is
// a table-valued function source rather than a real table: `left CROSS JOIN
// <source> AS alias`. It is the typed composition point promised
// in place of the earlier render-only JSON_TABLE helper and the deferred
// json_each/json_tree wiring.
//
// The source is correlated to the left table by construction: a SQLite
// json_each(json_col) / Postgres jsonb_array_elements(json_col) argument
// names a left-table column, and every supported dialect treats a FROM-clause
// table-valued function as implicitly LATERAL, so no LATERAL keyword is
// emitted. All/Stream scan one source row per left row in a SINGLE rows.Scan
// per result row, via the same scanJoinRow machinery the table joins use.
//
// Where/OrderBy scope to the left (A) table; WhereSource/OrderBySource scope
// to the source's columns (B), built from the source's typed handles. A CROSS
// join emits a row only when the source produces one, so B is always present;
// use LeftJoinTVF for the null-safe LEFT form.
type TVFJoin2[A any, PA ptrScanner[A], B any, PB ptrScanner[B]] struct {
	left        Query[A, PA]
	source      TableValuedSource[B]
	whereSource Predicate[B]
	orderSource []OrderTerm[B]
}

// JoinTVF starts a cross TVF builder from left over src. See TVFJoin2 for the
// result and correlation contract.
func JoinTVF[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](left Query[A, PA], src TableValuedSource[B]) TVFJoin2[A, PA, B, PB] {
	return TVFJoin2[A, PA, B, PB]{left: left, source: src}
}

// Where combines p into j's left-table filter with AND, matching
// Query[T, PT].Where's copy-on-write rule exactly.
func (j TVFJoin2[A, PA, B, PB]) Where(p Predicate[A]) TVFJoin2[A, PA, B, PB] {
	j.left = j.left.Where(p)

	return j
}

// WhereSource combines p into j's table-valued-source (B) filter with AND.
// Build p from the source's own typed column handles, which carry the
// derived-table alias. It is a separate method from Where -- rather than one
// Where accepting either Predicate[A] or Predicate[B] -- so A and B stay
// compile-time pinned at each call site (mirroring Join2.WhereRight).
func (j TVFJoin2[A, PA, B, PB]) WhereSource(p Predicate[B]) TVFJoin2[A, PA, B, PB] {
	if j.whereSource.IsSet() {
		j.whereSource = And(j.whereSource, p)
	} else {
		j.whereSource = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the LEFT (A) table only. Use
// OrderBySource for the source's columns.
func (j TVFJoin2[A, PA, B, PB]) OrderBy(terms ...OrderTerm[A]) TVFJoin2[A, PA, B, PB] {
	j.left = j.left.OrderBy(terms...)

	return j
}

// OrderBySource appends ORDER BY terms scoped to the table-valued source's
// output columns, mirroring Join2.OrderByRight. The two lists render as one
// ORDER BY clause, left terms first, each qualified to its own side.
func (j TVFJoin2[A, PA, B, PB]) OrderBySource(terms ...OrderTerm[B]) TVFJoin2[A, PA, B, PB] {
	next := make([]OrderTerm[B], 0, len(j.orderSource)+len(terms))
	next = append(next, j.orderSource...)
	next = append(next, terms...)
	j.orderSource = next

	return j
}

// Limit sets j's row limit.
func (j TVFJoin2[A, PA, B, PB]) Limit(n int) TVFJoin2[A, PA, B, PB] {
	j.left = j.left.Limit(n)

	return j
}

// Offset sets j's row offset.
func (j TVFJoin2[A, PA, B, PB]) Offset(n int) TVFJoin2[A, PA, B, PB] {
	j.left = j.left.Offset(n)

	return j
}

// render turns j's accumulated state into SQL text plus positional args. The
// source's own RenderSource performs the dialect capability gate, so a
// dialect that lacks the function family is rejected with the same typed
// error on every execution path before any SQL is issued.
func (j TVFJoin2[A, PA, B, PB]) render(d dialect.Dialect) (string, []any, error) {
	return renderTVFJoin[A, PA, B, PB](d, false, j.left, j.source, j.whereSource, j.orderSource)
}

// All runs j against exec and returns every matching row as a typed
// Row2[A, B], scanned via ONE rows.Scan call per row spanning both the left
// table's and the source's columns -- one round trip total.
func (j TVFJoin2[A, PA, B, PB]) All(ctx context.Context, exec db.DB) ([]Row2[A, B], error) {
	return lateralCollect(ctx, exec, "TVFJoin2.All", j.render,
		len(j.left.table.Columns()), len(j.source.SrcColumns()),
		j.scan())
}

// scan adapts the source's NewRow into the shared lateralCollect/lateralStream
// scan signature, so a TVF row is built from the source's own zero value.
func (j TVFJoin2[A, PA, B, PB]) scan() func(rows db.Rows, aCols, bCols int) (Row2[A, B], error) {
	newRow := j.source.NewRow

	return func(rows db.Rows, aCols, bCols int) (Row2[A, B], error) {
		return scanTVFJoinRow[A, PA, B, PB](rows, newRow, aCols, bCols)
	}
}

// Stream runs j against exec and yields every matching row one at a time, via
// iter.Seq2[Row2[A, B], error]. See Join2.Stream for the range-over-func
// cleanup guarantee.
func (j TVFJoin2[A, PA, B, PB]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row2[A, B], error] {
	return lateralStream(ctx, exec, "TVFJoin2.Stream", j.render,
		len(j.left.table.Columns()), len(j.source.SrcColumns()),
		j.scan())
}

// Explain runs j's TVF SELECT under EXPLAIN against exec; see Query.Explain
// for the passthrough contract. The dialect gate runs through the source's
// RenderSource, so an unsupported dialect is a typed ErrUnsupportedByDialect
// here too.
func (j TVFJoin2[A, PA, B, PB]) Explain(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, false, "TVFJoin2.Explain", j.render)
}

// ExplainAnalyze runs j's TVF SELECT under EXPLAIN ANALYZE against exec; see
// Query.ExplainAnalyze for the capability gate.
func (j TVFJoin2[A, PA, B, PB]) ExplainAnalyze(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, true, "TVFJoin2.Explain", j.render)
}

// LeftTVFJoin2 is the null-safe sibling of TVFJoin2 for
// `left LEFT JOIN <source> AS alias ON TRUE`: All/Stream return
// Row2[A, Option[B]], so an outer row whose source produces nothing comes
// back with Option[B]{}.IsSome() == false, never a same-shaped zero-valued B{}
// that could be mistaken for a real source row.
type LeftTVFJoin2[A any, PA ptrScanner[A], B any, PB ptrScanner[B]] struct {
	left        Query[A, PA]
	source      TableValuedSource[B]
	whereSource Predicate[B]
	orderSource []OrderTerm[B]
}

// LeftJoinTVF starts a left TVF builder from left over src. See LeftTVFJoin2
// for the null-safe result contract.
func LeftJoinTVF[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](left Query[A, PA], src TableValuedSource[B]) LeftTVFJoin2[A, PA, B, PB] {
	return LeftTVFJoin2[A, PA, B, PB]{left: left, source: src}
}

// Where combines p into j's left-table filter with AND.
func (j LeftTVFJoin2[A, PA, B, PB]) Where(p Predicate[A]) LeftTVFJoin2[A, PA, B, PB] {
	j.left = j.left.Where(p)

	return j
}

// WhereSource combines p into j's table-valued-source (B) filter with AND;
// see TVFJoin2.WhereSource.
func (j LeftTVFJoin2[A, PA, B, PB]) WhereSource(p Predicate[B]) LeftTVFJoin2[A, PA, B, PB] {
	if j.whereSource.IsSet() {
		j.whereSource = And(j.whereSource, p)
	} else {
		j.whereSource = p
	}

	return j
}

// OrderBy appends ORDER BY terms scoped to the LEFT (A) table only.
func (j LeftTVFJoin2[A, PA, B, PB]) OrderBy(terms ...OrderTerm[A]) LeftTVFJoin2[A, PA, B, PB] {
	j.left = j.left.OrderBy(terms...)

	return j
}

// OrderBySource appends ORDER BY terms scoped to the table-valued source's
// output columns; see TVFJoin2.OrderBySource.
func (j LeftTVFJoin2[A, PA, B, PB]) OrderBySource(terms ...OrderTerm[B]) LeftTVFJoin2[A, PA, B, PB] {
	next := make([]OrderTerm[B], 0, len(j.orderSource)+len(terms))
	next = append(next, j.orderSource...)
	next = append(next, terms...)
	j.orderSource = next

	return j
}

// Limit sets j's row limit.
func (j LeftTVFJoin2[A, PA, B, PB]) Limit(n int) LeftTVFJoin2[A, PA, B, PB] {
	j.left = j.left.Limit(n)

	return j
}

// Offset sets j's row offset.
func (j LeftTVFJoin2[A, PA, B, PB]) Offset(n int) LeftTVFJoin2[A, PA, B, PB] {
	j.left = j.left.Offset(n)

	return j
}

func (j LeftTVFJoin2[A, PA, B, PB]) render(d dialect.Dialect) (string, []any, error) {
	return renderTVFJoin[A, PA, B, PB](d, true, j.left, j.source, j.whereSource, j.orderSource)
}

// All runs j against exec and returns every matching row as a typed
// Row2[A, Option[B]], scanned via ONE rows.Scan call per row (see
// scanLeftJoinRow) -- one round trip total.
func (j LeftTVFJoin2[A, PA, B, PB]) All(ctx context.Context, exec db.DB) ([]Row2[A, Option[B]], error) {
	return lateralCollect(ctx, exec, "LeftTVFJoin2.All", j.render,
		len(j.left.table.Columns()), len(j.source.SrcColumns()),
		j.scan())
}

// scan adapts the source's NewRow into the shared scan signature for the
// null-safe LEFT form (an all-NULL source side becomes None).
func (j LeftTVFJoin2[A, PA, B, PB]) scan() func(rows db.Rows, aCols, bCols int) (Row2[A, Option[B]], error) {
	newRow := j.source.NewRow

	return func(rows db.Rows, aCols, bCols int) (Row2[A, Option[B]], error) {
		return scanLeftTVFJoinRow[A, PA, B, PB](rows, newRow, aCols, bCols)
	}
}

// Stream runs j against exec and yields every matching row one at a time, via
// iter.Seq2[Row2[A, Option[B]], error].
func (j LeftTVFJoin2[A, PA, B, PB]) Stream(ctx context.Context, exec db.DB) iter.Seq2[Row2[A, Option[B]], error] {
	return lateralStream(ctx, exec, "LeftTVFJoin2.Stream", j.render,
		len(j.left.table.Columns()), len(j.source.SrcColumns()),
		j.scan())
}

// Explain runs j's LEFT JOIN TVF SELECT under EXPLAIN against exec; see
// Query.Explain and TVFJoin2.Explain for the dialect gate.
func (j LeftTVFJoin2[A, PA, B, PB]) Explain(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, false, "LeftTVFJoin2.Explain", j.render)
}

// ExplainAnalyze runs j's LEFT JOIN TVF SELECT under EXPLAIN ANALYZE against
// exec; see Query.ExplainAnalyze for the capability gate.
func (j LeftTVFJoin2[A, PA, B, PB]) ExplainAnalyze(ctx context.Context, exec db.DB) ([]string, error) {
	return explain(ctx, exec, true, "LeftTVFJoin2.Explain", j.render)
}

// renderTVFJoin is the shared render body behind TVFJoin2 and LeftTVFJoin2:
// it validates the source, renders its function-call text through the
// source's own dialect gate, and hands everything to render.SelectTVF. err is
// a typed construction-time error (empty alias / no output columns) or the
// source's typed dialect.ErrUnsupportedByDialect, never invalid SQL.
func renderTVFJoin[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](
	d dialect.Dialect,
	leftJoin bool,
	left Query[A, PA],
	src TableValuedSource[B],
	whereSource Predicate[B],
	orderSource []OrderTerm[B],
) (string, []any, error) {
	if err := validateTVFSource[B](src); err != nil {
		return "", nil, err
	}

	sourceSQL, err := src.RenderSource(d)
	if err != nil {
		return "", nil, fmt.Errorf("orm: %w", err)
	}

	return render.SelectTVF(
		d,
		leftJoin,
		left.table.Name(), left.table.Columns(),
		sourceSQL, src.SrcAlias(), src.SrcColumns(),
		toRenderNode[A](left.where.Render()),
		toRenderNode[B](whereSource.Render()),
		toRenderOrder(left.order),
		toRenderOrder(orderSource),
		left.limit, left.offset,
	)
}

// scanTVFJoinRow scans one joined row into a Row2[A, B] via exactly one
// underlying rows.Scan call. It mirrors scanJoinRow, except B is built by the
// source's own NewRow rather than a bare `var b B` -- so a source that wants a
// non-zero default row controls its construction. aCols/bCols are the left
// table's and the source's column counts in projection order.
func scanTVFJoinRow[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](
	rows db.Rows, newRow func() B, aCols, bCols int,
) (Row2[A, B], error) {
	holders, dests := joinHolders(aCols + bCols)

	if err := rows.Scan(dests...); err != nil {
		return Row2[A, B]{}, fmt.Errorf("orm: tvf join scan: %w", err)
	}

	var a A

	if err := PA(&a).Scan(&rowFeed{vals: holders[:aCols]}); err != nil {
		return Row2[A, B]{}, fmt.Errorf("orm: tvf join scan: %w", err)
	}

	b := newRow()

	if err := PB(&b).Scan(&rowFeed{vals: holders[aCols:]}); err != nil {
		return Row2[A, B]{}, fmt.Errorf("orm: tvf join scan: %w", err)
	}

	return Row2[A, B]{A: a, B: b}, nil
}

// scanLeftTVFJoinRow scans one LEFT-JOINed row into a Row2[A, Option[B]] via
// exactly one underlying rows.Scan call. It mirrors scanLeftJoinRow (the
// source side is scanned into *any holders first and only converted to B when
// at least one is non-nil), with B built by the source's NewRow.
func scanLeftTVFJoinRow[A any, PA ptrScanner[A], B any, PB ptrScanner[B]](
	rows db.Rows, newRow func() B, aCols, bCols int,
) (Row2[A, Option[B]], error) {
	holders, dests := joinHolders(aCols + bCols)

	if err := rows.Scan(dests...); err != nil {
		return Row2[A, Option[B]]{}, fmt.Errorf("orm: left tvf join scan: %w", err)
	}

	var a A

	if err := PA(&a).Scan(&rowFeed{vals: holders[:aCols]}); err != nil {
		return Row2[A, Option[B]]{}, fmt.Errorf("orm: left tvf join scan: %w", err)
	}

	if allNil(holders[aCols:]) {
		return Row2[A, Option[B]]{A: a, B: None[B]()}, nil
	}

	b := newRow()

	if err := PB(&b).Scan(&rowFeed{vals: holders[aCols:]}); err != nil {
		return Row2[A, Option[B]]{}, fmt.Errorf("orm: left tvf join scan: %w", err)
	}

	return Row2[A, Option[B]]{A: a, B: Some(b)}, nil
}

// validateTVFSource rejects a source that cannot render a well-formed FROM
// entry: an empty alias would produce a dangling `AS`, and an empty column
// list would leave B's positional Scan with nothing to read -- both are
// construction-time bugs reported as typed errors rather than invalid SQL.
func validateTVFSource[B any](src TableValuedSource[B]) error {
	if src.SrcAlias() == "" {
		return errors.New("orm: TVFJoin2: table-valued source needs a non-empty alias")
	}

	if len(src.SrcColumns()) == 0 {
		return fmt.Errorf("orm: TVFJoin2: table-valued source %q has no output columns", src.SrcAlias())
	}

	return nil
}
