package orm

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// setOpKind identifies a set operation over two same-typed queries, mirroring
// render.SetOpOp's values (same underlying int representation).
type setOpKind int

// Supported set operators.
const (
	setOpUnion setOpKind = iota
	setOpUnionAll
	setOpIntersect
	setOpExcept
)

// SetOpQuery is an immutable, value-type set-operation builder combining
// two same-typed Query[T, PT] values, following the same copy-on-write
// chain discipline as Query[T, PT]. Its All/First/Count execute
//
//	SELECT ... FROM <left> <op> SELECT ... FROM <right>
//
// as one statement. OrderBy/Limit/Offset apply to the whole result and
// therefore reference the queries' output column names (bare, unqualified)
// -- exactly the OrderTerms Column.Asc()/Desc() already produce.
//
// UNION and UNION ALL are universal SQL and work on every supported
// dialect. INTERSECT and EXCEPT are gated behind dialect.SetOpDialect: a
// dialect lacking the capability returns a typed
// dialect.ErrUnsupportedByDialect at All/First/Count time, never a panic or
// a silent wrong-SQL fallback.
type SetOpQuery[T any, PT ptrScanner[T]] struct {
	op     setOpKind
	left   Query[T, PT]
	right  Query[T, PT]
	order  []OrderTerm[T]
	limit  int
	offset int
}

// Union combines left and right with UNION (deduplicating).
func Union[T any, PT ptrScanner[T]](left, right Query[T, PT]) SetOpQuery[T, PT] {
	return SetOpQuery[T, PT]{op: setOpUnion, left: left, right: right}
}

// UnionAll combines left and right with UNION ALL (preserving duplicates).
// It exists alongside Union because a recursive CTE's body requires UNION
// ALL (see WithRecursive), and it is the natural companion for result sets
// where deduplication is either not wanted or already guaranteed.
func UnionAll[T any, PT ptrScanner[T]](left, right Query[T, PT]) SetOpQuery[T, PT] {
	return SetOpQuery[T, PT]{op: setOpUnionAll, left: left, right: right}
}

// Intersect keeps only the rows present in both left and right
// (deduplicating). Requires dialect.SetOpDialect -- see SetOpQuery's doc
// comment.
func Intersect[T any, PT ptrScanner[T]](left, right Query[T, PT]) SetOpQuery[T, PT] {
	return SetOpQuery[T, PT]{op: setOpIntersect, left: left, right: right}
}

// Except keeps only the rows in left that are NOT in right (deduplicating).
// Requires dialect.SetOpDialect -- see SetOpQuery's doc comment.
func Except[T any, PT ptrScanner[T]](left, right Query[T, PT]) SetOpQuery[T, PT] {
	return SetOpQuery[T, PT]{op: setOpExcept, left: left, right: right}
}

// OrderBy appends ORDER BY terms applied to the whole set-operation
// result. It copies s.order into a FRESH backing array before appending,
// matching Query[T, PT].OrderBy's branch-safety rule.
func (s SetOpQuery[T, PT]) OrderBy(terms ...OrderTerm[T]) SetOpQuery[T, PT] {
	next := make([]OrderTerm[T], 0, len(s.order)+len(terms))
	next = append(next, s.order...)
	next = append(next, terms...)
	s.order = next

	return s
}

// Limit sets the result's row limit.
func (s SetOpQuery[T, PT]) Limit(n int) SetOpQuery[T, PT] {
	s.limit = n

	return s
}

// Offset sets the result's row offset.
func (s SetOpQuery[T, PT]) Offset(n int) SetOpQuery[T, PT] {
	s.offset = n

	return s
}

// requireSetOp reports whether d can run set op s. UNION/UNION ALL are
// universal; INTERSECT/EXCEPT require dialect.SetOpDialect with
// SupportsIntersectExcept reporting true (capability-table
// row: "INTERSECT/EXCEPT only 8.0.31+" for MySQL). Missing capability
// returns a typed dialect.ErrUnsupportedByDialect.
func requireSetOp(d dialect.Dialect, op setOpKind) error {
	if op != setOpIntersect && op != setOpExcept {
		return nil
	}

	sd, ok := d.(dialect.SetOpDialect)
	if !ok || !sd.SupportsIntersectExcept() {
		return fmt.Errorf("orm: %w: dialect %q does not support INTERSECT/EXCEPT", dialect.ErrUnsupportedByDialect, d.Name())
	}

	return nil
}

// All runs s against exec and returns every matching row, scanned via T's
// codegen'd Scan method.
func (s SetOpQuery[T, PT]) All(ctx context.Context, exec db.DB) ([]PT, error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	err = requireSetOp(d, s.op)
	if err != nil {
		return nil, err
	}

	query, args, err := render.SetOp(
		d,
		render.SetOpOp(s.op),
		s.left.table.Name(), s.left.table.Columns(), toRenderNode[T](s.left.where.Render()),
		s.right.table.Name(), s.right.table.Columns(), toRenderNode[T](s.right.where.Render()),
		toRenderOrder(s.order), s.limit, s.offset,
	)
	if err != nil {
		return nil, fmt.Errorf("orm: SetOpQuery.All: %w", err)
	}

	rows, err := queryRows(ctx, exec, query, encodeArgs(d, args))
	if err != nil {
		return nil, fmt.Errorf("orm: SetOpQuery.All: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []PT

	for rows.Next() {
		var v T

		p := PT(&v)
		if err := p.Scan(rows); err != nil {
			return nil, fmt.Errorf("orm: SetOpQuery.All: scan: %w", err)
		}

		out = append(out, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: SetOpQuery.All: %w", err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: SetOpQuery.All: %w", err)
	}

	return out, nil
}

// First runs s with an implicit LIMIT 1 and returns the first matching row,
// or ok=false if there was none.
func (s SetOpQuery[T, PT]) First(ctx context.Context, exec db.DB) (row PT, ok bool, err error) {
	rows, err := s.Limit(1).All(ctx, exec)
	if err != nil {
		return nil, false, err
	}

	if len(rows) == 0 {
		return nil, false, nil
	}

	return rows[0], true, nil
}

// Count runs `SELECT COUNT(*) FROM (<set op>)` and returns the number of
// rows the set operation produces. order/limit/offset are ignored (a count
// is over the whole result).
func (s SetOpQuery[T, PT]) Count(ctx context.Context, exec db.DB) (int64, error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return 0, err
	}

	err = requireSetOp(d, s.op)
	if err != nil {
		return 0, err
	}

	inner, args, err := render.SetOp(
		d,
		render.SetOpOp(s.op),
		s.left.table.Name(), s.left.table.Columns(), toRenderNode[T](s.left.where.Render()),
		s.right.table.Name(), s.right.table.Columns(), toRenderNode[T](s.right.where.Render()),
		nil, 0, 0,
	)
	if err != nil {
		return 0, fmt.Errorf("orm: SetOpQuery.Count: %w", err)
	}

	// The derived table must carry an alias: MySQL 8 (error 1248) and the
	// SQL standard both require every derived table to be named. A fixed
	// non-colliding name is safe on Postgres/SQLite too.
	rows, err := queryRows(ctx, exec, "SELECT COUNT(*) FROM ("+inner+") AS \"_orm_count\"", encodeArgs(d, args))
	if err != nil {
		return 0, fmt.Errorf("orm: SetOpQuery.Count: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var n int64

	if rows.Next() {
		if err := rows.Scan(&n); err != nil {
			return 0, fmt.Errorf("orm: SetOpQuery.Count: scan: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("orm: SetOpQuery.Count: %w", err)
	}

	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("orm: SetOpQuery.Count: %w", err)
	}

	return n, nil
}
