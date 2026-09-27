package orm

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// ErrProjectionEmpty is returned when a projected query is executed with no
// projections: an empty projection list would render `SELECT FROM ...`,
// which is invalid on every dialect, so it fails closed before rendering.
var ErrProjectionEmpty = errors.New("orm: projected query requires at least one projection")

// Projection is one aliased scalar output column of a ProjectedQuery: an
// expression tree (a plain column, a scalar function/CASE expression, or a
// scalar subquery) plus its output alias. Its fields are unexported; it is
// built by Expr.As, Column.As or NullableColumn.As, so a caller can never
// inject a raw SQL identifier or fragment through a projection.
type Projection[T any] struct {
	expr  Node
	alias string
}

// As aliases the expression as an output column, so it can be passed to
// Query.Project. An empty alias renders the bare expression (scanned
// positionally). It is the load-bearing entry point for expression
// projection.
func (e Expr[T, V]) As(alias string) Projection[T] {
	return Projection[T]{expr: e.n, alias: alias}
}

// As aliases a plain column as an output column for Query.Project.
func (c Column[T, V]) As(alias string) Projection[T] {
	return Projection[T]{expr: Node{Kind: NColumn, Table: c.table, Column: c.name}, alias: alias}
}

// As aliases a nullable column as an output column for Query.Project.
func (c NullableColumn[T, V]) As(alias string) Projection[T] {
	return Projection[T]{expr: Node{Kind: NColumn, Table: c.table, Column: c.name}, alias: alias}
}

// ScalarSubquery builds a scalar subquery expression: the inner query's
// single projected column, rendered inline as `(SELECT ...)` wherever it is
// used -- most usefully projected as an aliased output column via As. The
// inner query is snapshotted at construction time and rendered through the
// enclosing statement's dialect with its placeholder counter threaded
// through, exactly like the predicate subquery paths (see subquery.go). It
// must project exactly one column; a wider projection is a typed
// rendering-time error.
//
// T is the OUTER entity the expression belongs to and V the expression's
// value type; neither can be inferred from inner, so both are given
// explicitly at the call site:
//
//	orm.ScalarSubquery[widget, int64](From(orders).Columns(amount.Col()))
func ScalarSubquery[T any, V any, C any, PC ptrScanner[C]](inner Query[C, PC]) Expr[T, V] {
	return Expr[T, V]{n: Node{Kind: NSubquery, Value: toSubquery(inner)}}
}

// ProjectedRow is the scan surface a projected result row exposes. It mirrors
// database/sql's *sql.Row/*sql.Rows Scan contract (and db.Rows satisfies it
// structurally), so a callback scans each row positionally into its own
// destination variables -- the result of an expression projection is not an
// entity T and therefore has no generated Scan method.
type ProjectedRow interface {
	Scan(dest ...any) error
}

// ProjectedQuery is an immutable, value-type SELECT builder whose
// projection is an ordered list of aliased scalar expressions rather than
// entity T's columns. It is started from a Query[T, PT] via Project and
// followed by the same copy-on-write Where/OrderBy/Limit/Offset chain
// discipline. Because a projected row is not T, it is executed through the
// All/First scan surface, not All/Stream's []PT.
type ProjectedQuery[T any, PT ptrScanner[T]] struct {
	table       Table[T]
	projections []Projection[T]
	where       Predicate[T]
	order       []OrderTerm[T]
	limit       int
	offset      int
	distinct    bool
	lock        LockMode
	nowait      bool
	skipLocked  bool
}

// Project starts a projected query over q's table, selecting exactly
// projections (in order). It carries over q's existing Where/OrderBy/
// Limit/Offset and SELECT modifiers so a caller can build either order. The
// projections slice is copied, and the receiver is left unmodified.
func (q Query[T, PT]) Project(projections ...Projection[T]) ProjectedQuery[T, PT] {
	return ProjectedQuery[T, PT]{
		table:       q.table,
		projections: append([]Projection[T](nil), projections...),
		where:       q.where,
		order:       q.order,
		limit:       q.limit,
		offset:      q.offset,
		distinct:    q.distinct,
		lock:        q.lock,
		nowait:      q.nowait,
		skipLocked:  q.skipLocked,
	}
}

// Where combines p into the projected query's filter with AND, leaving the
// receiver unmodified -- the same copy-on-write rule Query.Where follows.
func (p ProjectedQuery[T, PT]) Where(pred Predicate[T]) ProjectedQuery[T, PT] {
	if p.where.IsSet() {
		p.where = And(p.where, pred)
	} else {
		p.where = pred
	}

	return p
}

// OrderBy appends terms to the projected query's ORDER BY list, copying into
// a fresh backing array.
func (p ProjectedQuery[T, PT]) OrderBy(terms ...OrderTerm[T]) ProjectedQuery[T, PT] {
	next := make([]OrderTerm[T], 0, len(p.order)+len(terms))
	next = append(next, p.order...)
	next = append(next, terms...)
	p.order = next

	return p
}

// Limit sets the projected query's row limit (0 renders no LIMIT).
func (p ProjectedQuery[T, PT]) Limit(n int) ProjectedQuery[T, PT] {
	p.limit = n

	return p
}

// Offset sets the projected query's row offset (0 renders no OFFSET).
func (p ProjectedQuery[T, PT]) Offset(n int) ProjectedQuery[T, PT] {
	p.offset = n

	return p
}

// Distinct makes the projected query render `SELECT DISTINCT`.
func (p ProjectedQuery[T, PT]) Distinct() ProjectedQuery[T, PT] {
	p.distinct = true

	return p
}

// selectModifiers erases p's SELECT modifiers into the renderer's shape.
func (p ProjectedQuery[T, PT]) selectModifiers() render.SelectModifiers {
	return render.SelectModifiers{
		Distinct:   p.distinct,
		Lock:       p.lock,
		NoWait:     p.nowait,
		SkipLocked: p.skipLocked,
	}
}

// validateSelect applies the same locking/modifier gates Query does before a
// projected statement is rendered.
func (p ProjectedQuery[T, PT]) validateSelect(d dialect.Dialect) error {
	return validateSelectModifiers(d, p.distinct, nil, p.lock, nil, p.nowait, p.skipLocked)
}

// toRenderProjections converts the typed projection list into render's erased
// shape, recursively erasing each expression tree (including any nested
// scalar subquery value).
func toRenderProjections[T any](ps []Projection[T]) []render.Projection {
	out := make([]render.Projection, len(ps))

	for i, p := range ps {
		out[i] = render.Projection{Expr: toRenderNode[T](p.expr), Alias: p.alias}
	}

	return out
}

// All runs p against exec and hands every result row to fn, returning the
// first row-callback error encountered. The db.Rows cursor is closed even if
// fn returns early. An empty projection list is ErrProjectionEmpty.
func (p ProjectedQuery[T, PT]) All(ctx context.Context, exec db.DB, fn func(row ProjectedRow) error) error {
	if len(p.projections) == 0 {
		return fmt.Errorf("orm: ProjectedQuery.All: %w", ErrProjectionEmpty)
	}

	d, err := resolveDialect(exec)
	if err != nil {
		return err
	}

	err = p.validateSelect(d)
	if err != nil {
		return err
	}

	query, args, err := render.ProjectedSelect(
		d,
		p.table.Name(),
		toRenderProjections[T](p.projections),
		toRenderNode[T](p.where.Render()),
		toRenderOrder(p.order),
		p.limit,
		p.offset,
		p.selectModifiers(),
	)
	if err != nil {
		return fmt.Errorf("orm: ProjectedQuery.All: %w", err)
	}

	encoded := encodeArgs(d, args)
	logQuery(query, encoded)

	rows, err := queryRows(ctx, exec, query, encoded)
	if err != nil {
		return fmt.Errorf("orm: ProjectedQuery.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		if err := fn(rows); err != nil {
			return fmt.Errorf("orm: ProjectedQuery.All: row: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("orm: ProjectedQuery.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return fmt.Errorf("orm: ProjectedQuery.All: %w", err)
	}

	return nil
}

// First runs p with an implicit LIMIT 1 and scans the first result row into
// dest, reporting ok=false when no row matches. An empty projection list is
// ErrProjectionEmpty.
func (p ProjectedQuery[T, PT]) First(ctx context.Context, exec db.DB, dest ...any) (ok bool, err error) {
	err = p.Limit(1).All(ctx, exec, func(row ProjectedRow) error {
		if scanErr := row.Scan(dest...); scanErr != nil {
			return scanErr
		}

		ok = true

		return nil
	})
	if err != nil {
		return false, err
	}

	return ok, nil
}
