package orm

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/render"
)

// WindowFunc identifies a scalar window function. Type alias for
// render.WindowFunc (orm imports render, never the reverse -- see orm.Op's
// doc comment for why aliasing beats a separately-kept-in-sync mirror).
type WindowFunc = render.WindowFunc

// Supported scalar window functions. These re-export render's
// identically-named constants.
const (
	WinNone      = render.WinNone
	WinRowNumber = render.WinRowNumber
	WinRank      = render.WinRank
	WinDenseRank = render.WinDenseRank
	WinLead      = render.WinLead
	WinLag       = render.WinLag
	WinNTile     = render.WinNTile
)

// FrameMode names a window frame's unit: ROWS, RANGE or GROUPS. FrameNone
// is the zero value -- a window expression never given a frame method
// renders no frame clause at all, matching the pre-frames behavior. Type
// alias for render.FrameMode; see WindowFunc's doc comment for why aliasing
// beats mirroring.
type FrameMode = render.FrameMode

// Supported window frame modes. These re-export render's identically-named
// constants.
const (
	FrameNone   = render.FrameNone
	FrameRows   = render.FrameRows
	FrameRange  = render.FrameRange
	FrameGroups = render.FrameGroups
)

// FrameBoundKind identifies which of the five SQL frame bounds a FrameBound
// is. Type alias for render.FrameBoundKind; see WindowFunc's doc comment
// for why aliasing beats mirroring.
type FrameBoundKind = render.FrameBoundKind

// The five SQL frame bounds. These re-export render's identically-named
// constants.
const (
	BoundUnboundedPreceding = render.BoundUnboundedPreceding
	BoundPreceding          = render.BoundPreceding
	BoundCurrentRow         = render.BoundCurrentRow
	BoundFollowing          = render.BoundFollowing
	BoundUnboundedFollowing = render.BoundUnboundedFollowing
)

// FrameBound is one endpoint of a window frame. N is the offset for
// Preceding/Following and is ignored by the other bounds.
type FrameBound struct {
	Kind FrameBoundKind
	N    int
}

// UnboundedPreceding returns the `UNBOUNDED PRECEDING` frame bound.
func UnboundedPreceding() FrameBound { return FrameBound{Kind: BoundUnboundedPreceding} }

// Preceding returns an `n PRECEDING` frame bound. n must be >= 0.
func Preceding(n int) FrameBound { return FrameBound{Kind: BoundPreceding, N: n} }

// CurrentRow returns the `CURRENT ROW` frame bound.
func CurrentRow() FrameBound { return FrameBound{Kind: BoundCurrentRow} }

// Following returns an `n FOLLOWING` frame bound. n must be >= 0.
func Following(n int) FrameBound { return FrameBound{Kind: BoundFollowing, N: n} }

// UnboundedFollowing returns the `UNBOUNDED FOLLOWING` frame bound.
func UnboundedFollowing() FrameBound { return FrameBound{Kind: BoundUnboundedFollowing} }

// overClause is one window function's OVER (PARTITION BY ... ORDER BY ...
// [ROWS|RANGE|GROUPS BETWEEN ... AND ...]) clause, held in its typed
// (T-erased) form until render time.
type overClause[T any] struct {
	partition  []AnyColumn[T]
	order      []OrderTerm[T]
	frameMode  FrameMode
	frameStart FrameBound
	frameEnd   FrameBound
}

// WindowExpr is a typed window expression for entity T: a scalar window
// function (RowNumber/Rank/DenseRank/Lead/Lag/NTile) or an aggregate over
// a window (SumOver(...).Over(...) etc.), rendered into a window query's
// SELECT list. Its Over method attaches the OVER clause; an expression
// never given one renders as `OVER ()` (the whole result set as a single
// window), which is valid SQL on every supported dialect.
//
// Naming decision (per "flag tension, propose the narrowest
// mitigation" instruction): the task brief sketches `Sum(...).Over(...)`,
// but Go has no generic methods -- a non-generic Aggregate (see agg.go's
// doc comment for why Aggregate is deliberately non-generic) cannot carry a
// type-parameterized Over, and agg.go's package-level `Sum`/`Count` names
// are already taken by the plain Aggregate constructors. The window
// aggregate constructors are therefore named SumOver/AvgOver/MinOver/
// MaxOver/CountOver, each returning a WindowExpr[T] whose `.Over(...)`
// method takes the partition/order terms -- the closest faithful spelling
// Go's type system allows.
type WindowExpr[T any] struct {
	fn    WindowFunc
	col   string
	value int
	alias string
	agg   *Aggregate
	over  overClause[T]
}

// RowNumber builds a ROW_NUMBER() window expression.
func RowNumber[T any]() WindowExpr[T] {
	return WindowExpr[T]{fn: WinRowNumber, alias: "row_number"}
}

// Rank builds a RANK() window expression (ties share a rank, with gaps).
func Rank[T any]() WindowExpr[T] {
	return WindowExpr[T]{fn: WinRank, alias: "rank"}
}

// DenseRank builds a DENSE_RANK() window expression (ties share a rank,
// without gaps).
func DenseRank[T any]() WindowExpr[T] {
	return WindowExpr[T]{fn: WinDenseRank, alias: "dense_rank"}
}

// Lead builds a LEAD(c) window expression: the value of c from the row
// "offset rows after" the current row within the window (default offset 1;
// a caller-specified offset is a future phase's concern). NULL when no such
// row exists -- scan it into an Option[V] for the last row of a window.
func Lead[T any, V any](c Column[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{fn: WinLead, col: name, alias: "lead_" + name}
}

// Lag builds a LAG(c) window expression: the value of c from the row
// "offset rows before" the current row within the window (default offset
// 1). NULL when no such row exists -- scan it into an Option[V] for the
// first row of a window.
func Lag[T any, V any](c Column[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{fn: WinLag, col: name, alias: "lag_" + name}
}

// NTile builds an NTILE(n) window expression: the bucket number (1..n) the
// current row falls into when the window's rows are split as evenly as
// possible.
func NTile[T any](n int) WindowExpr[T] {
	return WindowExpr[T]{fn: WinNTile, value: n, alias: "ntile"}
}

// CountOver builds a COUNT(*) OVER (...) aggregate-window expression.
func CountOver[T any]() WindowExpr[T] {
	return WindowExpr[T]{agg: &Aggregate{Func: AggCount}, alias: "count"}
}

// SumOver builds a SUM(c) OVER (...) aggregate-window expression. V is
// constrained to Numeric, matching Sum's constraint in agg.go.
func SumOver[T any, V Numeric](c Column[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{agg: &Aggregate{Func: AggSum, Column: name}, alias: "sum_" + name}
}

// AvgOver builds an AVG(c) OVER (...) aggregate-window expression.
func AvgOver[T any, V Numeric](c Column[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{agg: &Aggregate{Func: AggAvg, Column: name}, alias: "avg_" + name}
}

// MinOver builds a MIN(c) OVER (...) aggregate-window expression.
func MinOver[T any, V any](c Column[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{agg: &Aggregate{Func: AggMin, Column: name}, alias: "min_" + name}
}

// MaxOver builds a MAX(c) OVER (...) aggregate-window expression.
func MaxOver[T any, V any](c Column[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{agg: &Aggregate{Func: AggMax, Column: name}, alias: "max_" + name}
}

// SumNullableOver builds a SUM(c) OVER (...) expression over a nullable
// column; see SumNullable's rationale in agg.go.
func SumNullableOver[T any, V Numeric](c NullableColumn[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{agg: &Aggregate{Func: AggSum, Column: name}, alias: "sum_" + name}
}

// AvgNullableOver builds an AVG(c) OVER (...) expression over a nullable
// column.
func AvgNullableOver[T any, V Numeric](c NullableColumn[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{agg: &Aggregate{Func: AggAvg, Column: name}, alias: "avg_" + name}
}

// MinNullableOver builds a MIN(c) OVER (...) expression over a nullable
// column.
func MinNullableOver[T any, V any](c NullableColumn[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{agg: &Aggregate{Func: AggMin, Column: name}, alias: "min_" + name}
}

// MaxNullableOver builds a MAX(c) OVER (...) expression over a nullable
// column.
func MaxNullableOver[T any, V any](c NullableColumn[T, V]) WindowExpr[T] {
	name := c.Col().Name()

	return WindowExpr[T]{agg: &Aggregate{Func: AggMax, Column: name}, alias: "max_" + name}
}

// Over attaches the window clause to e: PARTITION BY partition, ORDER BY
// order. Both are optional; an empty Over renders `OVER ()`. e is copied
// and returned unmodified otherwise -- the same copy-on-write discipline
// the query builders follow, so reusing a base expression across branches
// is safe.
func (e WindowExpr[T]) Over(partition []AnyColumn[T], order []OrderTerm[T]) WindowExpr[T] {
	e.over.partition = append([]AnyColumn[T](nil), partition...)
	e.over.order = append([]OrderTerm[T](nil), order...)

	return e
}

// RowsBetween attaches a `ROWS BETWEEN start AND end` frame to e, counting
// physical rows. e is copied and returned, so a base expression can be
// reused across differently-framed branches. The frame is rendered after
// the OVER clause's PARTITION BY / ORDER BY. An unsupported dialect (see
// dialect.WindowFrameDialect) or a malformed bound is a typed rendering
// error at Scan time, never invalid SQL.
func (e WindowExpr[T]) RowsBetween(start, end FrameBound) WindowExpr[T] {
	e.over.frameMode = FrameRows
	e.over.frameStart = start
	e.over.frameEnd = end

	return e
}

// RangeBetween attaches a `RANGE BETWEEN start AND end` frame to e,
// counting peer rows by the ORDER BY value; see RowsBetween.
func (e WindowExpr[T]) RangeBetween(start, end FrameBound) WindowExpr[T] {
	e.over.frameMode = FrameRange
	e.over.frameStart = start
	e.over.frameEnd = end

	return e
}

// GroupsBetween attaches a `GROUPS BETWEEN start AND end` frame to e,
// counting peer groups; see RowsBetween. GROUPS is unsupported on MySQL and
// rejected with a typed dialect.ErrUnsupportedByDialect.
func (e WindowExpr[T]) GroupsBetween(start, end FrameBound) WindowExpr[T] {
	e.over.frameMode = FrameGroups
	e.over.frameStart = start
	e.over.frameEnd = end

	return e
}

func toRenderWindowExpr[T any](e WindowExpr[T]) render.WindowExpr {
	out := render.WindowExpr{
		Func:   e.fn,
		Column: e.col,
		Value:  e.value,
		Alias:  e.alias,
		Over: render.OverClause{
			Partition:  anyColumnNames(e.over.partition),
			Order:      toRenderOrder(e.over.order),
			FrameMode:  e.over.frameMode,
			FrameStart: render.FrameBound{Kind: e.over.frameStart.Kind, N: e.over.frameStart.N},
			FrameEnd:   render.FrameBound{Kind: e.over.frameEnd.Kind, N: e.over.frameEnd.N},
		},
	}

	if e.agg != nil {
		a := toRenderAggregate(*e.agg)
		out.Agg = &a
	}

	return out
}

func anyColumnNames[T any](cols []AnyColumn[T]) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Name()
	}

	return out
}

// WindowQuery is an immutable, value-type window-function SELECT builder
// over entity T, started from a Query[T, PT] via Select. It selects all of
// T's columns (in Table.Columns() order) followed by every window
// expression in Select order, and follows the same copy-on-write chain
// discipline as Query[T, PT].
//
// Scan takes a callback over the raw Row rather than returning a typed
// []PT, for the same reason GroupedQuery.Scan does (see its doc comment):
// a row of entity columns plus N heterogeneous window values does not
// correspond to any single T, so a callback is the only scan shape that
// avoids generating a bespoke result struct per call site.
type WindowQuery[T any, PT ptrScanner[T]] struct {
	table  Table[T]
	where  Predicate[T]
	exprs  []WindowExpr[T]
	order  []OrderTerm[T]
	limit  int
	offset int
}

// Select starts a window query over q, selecting all of T's columns plus
// every window expression. The resulting WindowQuery is queryable via Scan.
func (q Query[T, PT]) Select(exprs ...WindowExpr[T]) WindowQuery[T, PT] {
	next := make([]WindowExpr[T], len(exprs))
	copy(next, exprs)

	return WindowQuery[T, PT]{table: q.table, where: q.where, exprs: next}
}

// Where combines p into w's filter with AND -- the receiver is left
// unmodified, matching Query[T, PT].Where's rule exactly.
func (w WindowQuery[T, PT]) Where(p Predicate[T]) WindowQuery[T, PT] {
	if w.where.IsSet() {
		w.where = And(w.where, p)
	} else {
		w.where = p
	}

	return w
}

// OrderBy appends terms to w's ORDER BY list (the result's row ordering;
// a window expression's own intra-window ordering is set via Over). It
// copies w.order into a FRESH backing array before appending.
func (w WindowQuery[T, PT]) OrderBy(terms ...OrderTerm[T]) WindowQuery[T, PT] {
	next := make([]OrderTerm[T], 0, len(w.order)+len(terms))
	next = append(next, w.order...)
	next = append(next, terms...)
	w.order = next

	return w
}

// Limit sets w's row limit.
func (w WindowQuery[T, PT]) Limit(n int) WindowQuery[T, PT] {
	w.limit = n

	return w
}

// Offset sets w's row offset.
func (w WindowQuery[T, PT]) Offset(n int) WindowQuery[T, PT] {
	w.offset = n

	return w
}

// Scan runs w against exec and hands every result row to fn. Result
// columns are T's columns in Table.Columns() order, followed by each window
// expression in Select order -- the scan order every call site must read.
func (w WindowQuery[T, PT]) Scan(ctx context.Context, exec db.DB, fn func(row Row) error) error {
	d, err := resolveDialect(exec)
	if err != nil {
		return err
	}

	exprs := make([]render.WindowExpr, len(w.exprs))
	for i, e := range w.exprs {
		exprs[i] = toRenderWindowExpr(e)
	}

	query, args, err := render.WindowSelect(d, w.table.Name(), w.table.Columns(), exprs, toRenderNode[T](w.where.Render()), toRenderOrder(w.order), w.limit, w.offset)
	if err != nil {
		return fmt.Errorf("orm: WindowQuery.Scan: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return fmt.Errorf("orm: WindowQuery.Scan: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		if err := fn(rows); err != nil {
			_ = rows.Close()

			return fmt.Errorf("orm: WindowQuery.Scan: row: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: WindowQuery.Scan: %w", err)
	}

	if err := rows.Close(); err != nil {
		return fmt.Errorf("orm: WindowQuery.Scan: %w", err)
	}

	return nil
}
