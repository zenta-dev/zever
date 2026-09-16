package orm

import (
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// Columns explicitly sets the INSERT's target column list, in order. It is
// copy-on-write like every other builder method: the receiver is left
// unmodified and the returned Insert carries a fresh column slice.
//
// For a .Values insert the first Values call already fixes the column list,
// so Columns is mainly the explicit escape hatch for an INSERT ... SELECT
// (see Select): with no literal rows there is no first Values call to infer
// the list from. When Columns is not called, Select infers the target list
// from the source SELECT's own projection (which is always a deterministic
// list of T's columns), so Columns is required only when the target order
// must differ from the projection.
func (i Insert[T]) Columns(cols ...AnyColumn[T]) Insert[T] {
	names := make([]string, len(cols))
	for j, c := range cols {
		names[j] = c.Name()
	}

	i.columns = names

	return i
}

// Select turns the Insert into `INSERT INTO <table> (<columns>) <select>`,
// taking its rows from q instead of literal Values rows -- bulk copy,
// archive and transform. q is a Query over the SAME entity type T, so its
// projection is always a compile-time-known list of T's columns; the
// source SELECT is rendered through the same dialect (and render.Select
// path) as a standalone query, with the Postgres `$N` numbering continuing
// into any ON CONFLICT DO UPDATE SET assignments.
//
// The target column list is i.columns when Columns was called, else the
// source SELECT's projection (see Columns). Select composes with
// OnConflict/DoUpdate/DoNothing (see upsert.go) and Returning/
// ExecReturning; the conflict syntax is the same ON CONFLICT (Postgres/
// SQLite) the Values path uses, and the RETURNING capability restrictions
// are unchanged.
//
// Select and Values are mutually exclusive: calling Select after Values (or
// vice versa) is rejected at exec time rather than silently picking a
// source. A target column count that disagrees with the SELECT projection is
// likewise rejected at render time.
func (i Insert[T]) Select[PT ptrScanner[T]](q Query[T, PT]) Insert[T] {
	src := render.SelectSource{
		Table:   q.table.Name(),
		Columns: q.selectColumns(),
		Where:   toRenderNode[T](q.where.Render()),
		Order:   toRenderOrder(q.order),
		Limit:   q.limit,
		Offset:  q.offset,
	}

	i.selectSrc = &src

	return i
}

// renderInsertSelect renders an Insert whose rows come from a SELECT source
// (see Select). It validates the mutually-exclusive source and the target/
// projection column counts, resolves the target column list, and dispatches
// to the plain or ON CONFLICT renderer exactly as
// renderInsert does for the Values path.
func (i Insert[T]) renderInsertSelect(d dialect.Dialect) (query string, args []any, err error) {
	if i.rowsLen > 0 {
		return "", nil, errors.New("orm: Insert.Select: Select is mutually exclusive with Values rows")
	}

	columns := i.columns
	if len(columns) == 0 {
		// No explicit Columns: the SELECT's projection is the target list.
		columns = i.selectSrc.Columns
	}

	if len(columns) == 0 {
		return "", nil, errors.New("orm: Insert.Select: no target columns (the SELECT projects none; call Columns)")
	}

	if len(columns) != len(i.selectSrc.Columns) {
		return "", nil, fmt.Errorf(
			"orm: Insert.Select: %d target columns but the SELECT projects %d",
			len(columns), len(i.selectSrc.Columns),
		)
	}

	if i.conflict == nil {
		q, a, err := render.InsertSelect(d, i.table.Name(), columns, *i.selectSrc, i.returning)
		if err != nil {
			return "", nil, err
		}

		return q, a, nil
	}

	if i.conflict.action == conflictDoUpdate && len(i.conflict.sets) == 0 {
		return "", nil, errors.New("orm: upsert DO UPDATE requires at least one Set assignment")
	}

	// Same partial-upsert-WHERE capability gate as the Values path: an
	// unsupported predicate is a typed dialect.ErrUnsupportedByDialect, never
	// silently dropped.
	if err := i.requireConflictWhere(d); err != nil {
		return "", nil, err
	}

	var sets []render.Assignment
	if i.conflict.action == conflictDoUpdate {
		sets = toRenderAssignments(i.conflict.sets)
	}

	where := render.ConflictWhere{
		Target: toRenderNode[T](i.conflict.targetWhere),
		Update: toRenderNode[T](i.conflict.updateWhere),
	}

	return render.InsertSelectOnConflictWhere(d, i.table.Name(), columns, *i.selectSrc, i.conflict.target, where, sets, i.returning)
}
