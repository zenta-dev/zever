package orm

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// Set builds a non-NULL Assignment for a non-nullable Column c, for use
// with Insert[T].Values/Update[T].Set. Assignment[T] itself is already
// declared in orm/column.go (built there for NullableColumn.SetValue/
// SetNull); Set is the equivalent constructor for a plain Column[T,V],
// completing the "every Assignment[T] carries a real column name plus a
// driver-bindable value, never a bare string key" contract for BOTH
// column kinds. It is a free function (not a Column[T,V] method) for the
// same reason Contains/StartsWith/EndsWith in orm/column.go are free
// functions -- Go's generic receiver syntax only accepts type-parameter
// NAMES, and more importantly this keeps the Phase 2 mutation surface
// confined to this file rather than reopening orm/column.go. Set reaches
// into Column's unexported fields directly since it lives in the same
// package.
func Set[T any, V any](c Column[T, V], v V) Assignment[T] {
	return Assignment[T]{Column: c.Col(), Value: v}
}

// toRenderAssignments converts sets to render's own erased Assignment
// shape with a plain field-for-field literal, mirroring toRenderNode/
// toRenderOrder's existing conversion pattern in orm/query.go.
func toRenderAssignments[T any](sets []Assignment[T]) []render.Assignment {
	out := make([]render.Assignment, len(sets))

	for i, s := range sets {
		out[i] = render.Assignment{Column: s.Column.Name(), Value: s.Value}
	}

	return out
}

// Insert is an immutable, value-type SQL INSERT builder over entity T.
// Like Query[T] (see orm/query.go's doc comment), every chain method
// returns a NEW Insert value rather than mutating the receiver. Values
// stores its accumulated rows in a persistent linked list (insertRows), so
// appending one row is O(1) and allocates only the new node; nodes are
// never mutated after creation, so branching an Insert chain is safe by
// construction. OnConflict/Returning (orm/upsert.go) extend this shape into
// an upsert with an optional RETURNING clause.
type Insert[T any] struct {
	table         Table[T]
	columns       []string
	rows          *insertRows
	rowsLen       int
	conflict      *conflictSpec[T]
	returning     []string
	selectSrc     *render.SelectSource
	defaultValues bool
}

// insertRows is the immutable, singly-linked row list an Insert[T] chain
// builds. Each Values call prepends a new node carrying that call's value
// slice, so appending is O(1); nodes are never mutated after creation, so
// two Insert values branched from one base share the base's nodes without
// any risk of corruption -- the copy-on-write branch-safety contract of
// , satisfied by construction instead of by per-append copies
// (a 1000-row linear chain no longer pays O(n^2) copy work).
type insertRows struct {
	values []any
	prev   *insertRows
}

// rowsSlice materializes the chain's rows in declaration order (the chain
// is stored head-last). It allocates one [][]any header and reuses each
// row's value slice, so it is O(n) once, at Exec/render time, rather than
// O(n^2) spread across n Values calls.
func (i Insert[T]) rowsSlice() [][]any {
	out := make([][]any, 0, i.rowsLen)

	for r := i.rows; r != nil; r = r.prev {
		out = append(out, r.values)
	}

	for l, r := 0, len(out)-1; l < r; l, r = l+1, r-1 {
		out[l], out[r] = out[r], out[l]
	}

	return out
}

// InsertInto builds an Insert over t with no rows yet.
func InsertInto[T any](t Table[T]) Insert[T] {
	return Insert[T]{table: t}
}

// Values appends one row built from assignments. The FIRST call to Values
// on a given chain fixes i's column list, in the order assignments are
// given; every subsequent Values call on that same chain is expected to
// supply assignments for the same columns in the same order -- Values
// does not re-sort or reconcile differing column sets across rows,
// matching render.InsertMany's positional row contract. See
// ExampleInsert_Values for a runnable example.
func (i Insert[T]) Values(assignments ...Assignment[T]) Insert[T] {
	vals := make([]any, len(assignments))

	for idx, a := range assignments {
		vals[idx] = a.Value
	}

	if i.columns == nil {
		cols := make([]string, len(assignments))

		for idx, a := range assignments {
			cols[idx] = a.Column.Name()
		}

		i.columns = cols
	}

	i.rows = &insertRows{values: vals, prev: i.rows}
	i.rowsLen++

	return i
}

// DefaultValues marks the insert as an explicit
// `INSERT INTO <table> DEFAULT VALUES`: every column takes its server-side
// default (or NULL when it has none). It is mutually exclusive with Values
// and Select -- combining them is a typed error at Exec/render time rather
// than a silently-picked source (see renderInsert). DefaultValues composes
// with Returning/ExecReturning, so a row built entirely from column defaults
// can still be read back on Postgres/SQLite.
//
// An Insert with no Values row at all already renders DEFAULT VALUES (see
// Exec); DefaultValues exists to make that intent explicit and, crucially,
// to let a later Values/Select/Columns call be rejected instead of silently
// overriding the default-only insert.
func (i Insert[T]) DefaultValues() Insert[T] {
	i.defaultValues = true

	return i
}

// Exec runs i against exec: a single-row INSERT if exactly one row of
// Values was supplied, a multi-row INSERT if more than one, or
// `INSERT INTO <table> DEFAULT VALUES` if Values was never called. An
// OnConflict upsert renders its ON CONFLICT / ON DUPLICATE KEY UPDATE
// clause instead. Exec returns an error if Returning was requested -- a
// RETURNING statement returns rows and must run through ExecReturning
// instead, never silently dropping the clause.
func (i Insert[T]) Exec(ctx context.Context, exec db.DB) error {
	if len(i.returning) > 0 {
		return errors.New("orm: Insert.Exec: RETURNING requested via Returning(); use ExecReturning")
	}

	d, err := resolveDialect(exec)
	if err != nil {
		return err
	}

	query, args, err := i.renderInsert(d)
	if err != nil {
		return err
	}

	encoded := encodeArgs(d, args)
	logQuery(query, encoded)

	if _, err := execQuery(ctx, exec, query, encoded); err != nil {
		return fmt.Errorf("orm: Insert.Exec: %w", err)
	}

	return nil
}

// Update is an immutable, value-type SQL UPDATE builder over entity T,
// following the same copy-on-write chain discipline as Query[T]. OrderBy/
// Limit/Offset append an optional `ORDER BY ... [LIMIT ... [OFFSET ...]]`
// tail to the statement (capability-gated per dialect; see
// dialect.MutateOrderDialect -- no in-tree dialect supports a mutation
// tail). Returning
// adds an optional RETURNING clause with the same semantics (column-order
// defaults, rejection on dialects without RETURNING support) as
// Insert.Returning.
type Update[T any] struct {
	table     Table[T]
	where     Predicate[T]
	sets      []Assignment[T]
	joins     []render.JoinSpec
	joinType  JoinType
	order     []OrderTerm[T]
	limit     int
	offset    int
	returning []string
}

// UpdateTable builds an Update over t with no filter/assignments set.
func UpdateTable[T any](t Table[T]) Update[T] {
	return Update[T]{table: t}
}

// joinRelation is the parent-pinned, type-erased view of a Relation that
// Update[T].Join and Delete[T].Join accept. The Table[T] parameter is what
// pins the relation's Parent type parameter to T at compile time: only
// Relation[T, C] exposes a joinSpec(Table[T]) method, so a relation whose
// parent is some other entity does not satisfy this interface. The erased
// render.JoinSpec is what the renderer actually needs, since a joined
// UPDATE/DELETE never scans the child side.
type joinRelation[T any] interface {
	joinSpec(Table[T]) render.JoinSpec
}

func (r Relation[Parent, Child]) joinSpec(_ Table[Parent]) render.JoinSpec {
	return render.JoinSpec{ParentCol: r.parentCol, ChildTable: r.childTable.Name(), ChildCol: r.childCol}
}

// Join appends a typed equi-join to the UPDATE, turning it into a joined
// UPDATE: `UPDATE t SET ... FROM <joined> WHERE ...` on Postgres/SQLite
// (other dialects with join support render their own JOIN form; see
// render.UpdateJoin). rel's Parent must be u's own entity type -- a
// Relation whose
// parent is a different entity does not compile. The join is INNER by the
// semantics of the FROM/USING form; LeftJoin additionally requires
// SupportsLeftMutateJoin (see dialect.MutateJoinDialect) and is rejected
// with a typed dialect.ErrUnsupportedByDialect at Exec time where
// unsupported. Join copies u.joins
// into a FRESH backing array before appending, matching Set's
// branch-safety rule.
func (u Update[T]) Join(rel joinRelation[T], joinType JoinType) Update[T] {
	spec := rel.joinSpec(u.table)

	next := make([]render.JoinSpec, 0, len(u.joins)+1)
	next = append(next, u.joins...)
	next = append(next, spec)
	u.joins = next
	u.joinType = joinType

	return u
}

// Where combines p into u's existing filter with AND -- the receiver is
// left unmodified, matching Query[T].Where's rule exactly.
func (u Update[T]) Where(p Predicate[T]) Update[T] {
	if u.where.IsSet() {
		u.where = And(u.where, p)
	} else {
		u.where = p
	}

	return u
}

// Set appends assignments to u's SET list. It copies u.sets into a FRESH
// backing array before appending -- never a bare append(u.sets, ...) --
// matching Query[T].OrderBy's branch-safety rule.
func (u Update[T]) Set(assignments ...Assignment[T]) Update[T] {
	next := make([]Assignment[T], 0, len(u.sets)+len(assignments))
	next = append(next, u.sets...)
	next = append(next, assignments...)
	u.sets = next

	return u
}

// OrderBy appends terms to u's ORDER BY list, turning the UPDATE into
// `UPDATE ... ORDER BY ...`. It copies u.order into a FRESH backing array
// before appending -- never a bare append(u.order, ...) -- matching
// Query[T].OrderBy's branch-safety rule. The tail is capability-gated at
// Exec time: no in-tree dialect supports it, so on Postgres or SQLite (or
// on a joined UPDATE on a dialect without joined support) Exec returns a
// typed dialect.ErrUnsupportedByDialect instead of rendering SQL the engine
// would reject.
func (u Update[T]) OrderBy(terms ...OrderTerm[T]) Update[T] {
	next := make([]OrderTerm[T], 0, len(u.order)+len(terms))
	next = append(next, u.order...)
	next = append(next, terms...)
	u.order = next

	return u
}

// Limit caps how many rows the UPDATE changes. A value of 0 (the zero
// value, also what an unset Limit produces) renders NO LIMIT clause --
// unlimited rows -- matching the Query[T].Limit zero-value-is-unset
// convention. On a dialect whose engine rejects LIMIT on UPDATE, Exec
// returns a typed dialect.ErrUnsupportedByDialect.
func (u Update[T]) Limit(n int) Update[T] {
	u.limit = n

	return u
}

// Offset skips the first n rows before the UPDATE changes any. A value of
// 0 (the zero value) renders no OFFSET clause, matching Query[T].Offset.
// OFFSET on UPDATE is not supported by any in-tree dialect (no in-tree
// engine accepts a DML tail), so Exec returns a typed
// dialect.ErrUnsupportedByDialect whenever n > 0.
func (u Update[T]) Offset(n int) Update[T] {
	u.offset = n

	return u
}

// requireMutateJoin reports whether d can run a joined UPDATE/DELETE with
// the requested joinType: the dialect.MutateJoinDialect capability must be
// present, the per-statement support bool (SupportsUpdateJoin /
// SupportsDeleteJoin) must report true, and a LEFT join additionally
// requires SupportsLeftMutateJoin. A missing capability returns a typed
// dialect.ErrUnsupportedByDialect, never a panic or silently rendering SQL
// the dialect rejects.
func requireMutateJoin(d dialect.Dialect, joinType JoinType, isUpdate bool) error {
	md, ok := d.(dialect.MutateJoinDialect)
	if !ok {
		return fmt.Errorf("orm: %w: dialect %q does not support joined UPDATE/DELETE", dialect.ErrUnsupportedByDialect, d.Name())
	}

	feature := "UPDATE"
	ok = md.SupportsUpdateJoin()

	if !isUpdate {
		feature = "DELETE"
		ok = md.SupportsDeleteJoin()
	}

	if !ok {
		return fmt.Errorf("orm: %w: dialect %q does not support %s with a join", dialect.ErrUnsupportedByDialect, d.Name(), feature)
	}

	if joinType == LeftJoin && !md.SupportsLeftMutateJoin() {
		return fmt.Errorf("orm: %w: dialect %q does not support LEFT JOIN in %s", dialect.ErrUnsupportedByDialect, d.Name(), feature)
	}

	return nil
}

// requireMutateOrder reports whether d can run an UPDATE/DELETE carrying an
// ORDER BY / LIMIT / OFFSET tail: the dialect.MutateOrderDialect capability
// must be present and the per-statement support bool must report true.
// hasOrder/limit/offset describe the statement's tail; joined indicates a
// multi-table statement (which additionally requires
// SupportsJoinedMutateOrderLimit). When the tail is
// entirely unset (no order, limit <= 0, offset <= 0) nothing is gated --
// the same zero-value-is-unset rule that keeps a plain UPDATE from needing
// a capability. A missing capability returns a typed
// dialect.ErrUnsupportedByDialect, never a panic or silently rendering SQL
// the dialect rejects.
func requireMutateOrder(d dialect.Dialect, hasOrder bool, limit, offset int, joined, isUpdate bool) error {
	if !hasOrder && limit <= 0 && offset <= 0 {
		return nil
	}

	mo, ok := d.(dialect.MutateOrderDialect)
	if !ok {
		return fmt.Errorf("orm: %w: dialect %q does not support UPDATE/DELETE ORDER BY/LIMIT", dialect.ErrUnsupportedByDialect, d.Name())
	}

	feature := "UPDATE"
	if !isUpdate {
		feature = "DELETE"
	}

	if joined {
		if !mo.SupportsJoinedMutateOrderLimit() {
			return fmt.Errorf("orm: %w: dialect %q does not support ORDER BY/LIMIT on a joined %s", dialect.ErrUnsupportedByDialect, d.Name(), feature)
		}

		if offset > 0 && !mo.SupportsMutateOffset() {
			return fmt.Errorf("orm: %w: dialect %q does not support OFFSET in %s", dialect.ErrUnsupportedByDialect, d.Name(), feature)
		}

		return nil
	}

	support := mo.SupportsUpdateOrderLimit()
	if !isUpdate {
		support = mo.SupportsDeleteOrderLimit()
	}

	if !support {
		return fmt.Errorf("orm: %w: dialect %q does not support ORDER BY/LIMIT in %s", dialect.ErrUnsupportedByDialect, d.Name(), feature)
	}

	if offset > 0 && !mo.SupportsMutateOffset() {
		return fmt.Errorf("orm: %w: dialect %q does not support OFFSET in %s", dialect.ErrUnsupportedByDialect, d.Name(), feature)
	}

	return nil
}

// Returning appends a `RETURNING <columns>` clause to u, turning the
// UPDATE into one that returns the updated rows instead of only writing
// them. Returning() with no arguments returns every column of T in codegen
// order, so ExecReturning's positional scan into T lines up exactly; an
// explicit column subset is allowed but is then the caller's responsibility
// to keep aligned with T's codegen'd Scan method.
//
// RETURNING is supported on Postgres and SQLite. On a dialect without
// RETURNING support, ExecReturning fails with a typed
// dialect.ErrUnsupportedByDialect at execution time -- the clause is never
// silently dropped.
func (u Update[T]) Returning(cols ...AnyColumn[T]) Update[T] {
	if len(cols) == 0 {
		u.returning = u.table.Columns()

		return u
	}

	cols2 := make([]string, len(cols))
	for j, c := range cols {
		cols2[j] = c.Name()
	}

	u.returning = cols2

	return u
}

// requireStatementCapability checks the per-statement capability u needs:
// a joined UPDATE requires dialect.MutateJoinDialect support (see
// requireMutateJoin), and an ORDER BY / LIMIT / OFFSET tail requires
// dialect.MutateOrderDialect support (see requireMutateOrder). A plain
// UPDATE needs nothing beyond the base dialect, and RETURNING is checked
// separately by ExecReturning.
func (u Update[T]) requireStatementCapability(d dialect.Dialect) error {
	if len(u.joins) > 0 {
		if err := requireMutateJoin(d, u.joinType, true); err != nil {
			return err
		}

		return requireMutateOrder(d, len(u.order) > 0, u.limit, u.offset, true, true)
	}

	return requireMutateOrder(d, len(u.order) > 0, u.limit, u.offset, false, true)
}

// renderStatement renders u for d, dispatching the plain (shape-cached
// render.Update) vs joined (render.UpdateJoin) UPDATE syntax and appending
// the ORDER BY / LIMIT / OFFSET tail and the RETURNING clause when one was
// requested. The capability gates -- mutate-join, mutate-order and
// returning -- are checked by the caller before this is reached; this only
// picks the renderer.
func (u Update[T]) renderStatement(d dialect.Dialect) (query string, args []any, err error) {
	where := toRenderNode[T](u.where.Render())
	sets := toRenderAssignments(u.sets)
	order := toRenderOrder(u.order)

	if len(u.joins) > 0 {
		if len(u.returning) > 0 {
			return render.UpdateJoinReturning(d, u.table.Name(), sets, u.joins, u.joinType, where, order, u.limit, u.offset, u.returning)
		}

		return render.UpdateJoin(d, u.table.Name(), sets, u.joins, u.joinType, where, order, u.limit, u.offset)
	}

	if len(u.returning) > 0 {
		return render.UpdateReturning(d, u.table.Name(), sets, where, order, u.limit, u.offset, u.returning)
	}

	return render.Update(d, u.table.Name(), sets, where, order, u.limit, u.offset)
}

// Exec runs u against exec and returns the number of rows affected. Exec
// returns an error if Returning was requested -- a RETURNING statement
// returns rows and must run through ExecReturning instead, never silently
// dropping the clause.
func (u Update[T]) Exec(ctx context.Context, exec db.DB) (int64, error) {
	if len(u.returning) > 0 {
		return 0, errors.New("orm: Update.Exec: RETURNING requested via Returning(); use ExecReturning")
	}

	if len(u.sets) == 0 {
		return 0, errors.New("orm: Update.Exec: no assignments (call Set)")
	}

	d, err := resolveDialect(exec)
	if err != nil {
		return 0, err
	}

	err = u.requireStatementCapability(d)
	if err != nil {
		return 0, err
	}

	query, args, err := u.renderStatement(d)
	if err != nil {
		return 0, fmt.Errorf("orm: Update.Exec: %w", err)
	}

	encoded := encodeArgs(d, args)
	logQuery(query, encoded)

	n, err := execQuery(ctx, exec, query, encoded)
	if err != nil {
		return 0, fmt.Errorf("orm: Update.Exec: %w", err)
	}

	return n, nil
}

// ExecReturning runs u with its RETURNING clause and returns the updated
// row(s), scanned via T's codegen'd Scan method -- the same positional
// scan machinery Query.All (and Insert.ExecReturning) uses, through the
// shared execReturning tail. callers write
//
//	rows, err := UpdateTable(widgets).
//		Where(widgetID.Eq("w1")).
//		Set(Set(widgetName, "Renamed")).
//		Returning().
//		ExecReturning(ctx, conn)
//
// and rows is []*widget. RETURNING reports each row's POST-write values:
// SET columns carry their new value, un-SET columns their pre-update one.
//
// ExecReturning requires dialect.ReturningDialect.SupportsReturning to
// report true; on a dialect without it, it returns a typed
// dialect.ErrUnsupportedByDialect. An UPDATE that matches no rows produces
// no RETURNING rows.
func (u Update[T]) ExecReturning[PT ptrScanner[T]](ctx context.Context, exec db.DB) ([]PT, error) {
	if len(u.returning) == 0 {
		return nil, errors.New("orm: Update.ExecReturning: no RETURNING columns; call Returning() first")
	}

	if len(u.sets) == 0 {
		return nil, errors.New("orm: Update.ExecReturning: no assignments (call Set)")
	}

	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	err = requireReturning(d)
	if err != nil {
		return nil, err
	}

	err = u.requireStatementCapability(d)
	if err != nil {
		return nil, err
	}

	query, args, err := u.renderStatement(d)
	if err != nil {
		return nil, fmt.Errorf("orm: Update.ExecReturning: %w", err)
	}

	return execReturning[T, PT](ctx, exec, d, query, args, "Update.ExecReturning")
}

// Delete is an immutable, value-type SQL DELETE builder over entity T,
// following the same copy-on-write chain discipline as Query[T]. OrderBy/
// Limit/Offset append an optional `ORDER BY ... [LIMIT ... [OFFSET ...]]`
// tail to the statement (capability-gated per dialect; see
// dialect.MutateOrderDialect -- no in-tree dialect supports a mutation
// tail). Returning
// adds an optional RETURNING clause with the same semantics (column-order
// defaults, rejection on dialects without RETURNING support) as
// Insert.Returning.
type Delete[T any] struct {
	table     Table[T]
	where     Predicate[T]
	joins     []render.JoinSpec
	joinType  JoinType
	order     []OrderTerm[T]
	limit     int
	offset    int
	returning []string
}

// DeleteFrom builds a Delete over t with no filter set. Exec-ing it with
// no Where deletes every row -- the caller's decision to make, not the
// renderer's (mirrors render.Delete's documented behavior).
func DeleteFrom[T any](t Table[T]) Delete[T] {
	return Delete[T]{table: t}
}

// Join appends a typed equi-join to the DELETE, turning it into a joined
// DELETE: `DELETE FROM t USING <joined> WHERE ...` on
// Postgres (other dialects with join support render their own JOIN form;
// see render.DeleteJoin).
// SQLite has no DELETE...USING, so Exec returns a typed
// dialect.ErrUnsupportedByDialect there. rel's Parent must be del's own
// entity type. LeftJoin additionally requires SupportsLeftMutateJoin (see
// dialect.MutateJoinDialect) and is rejected with a typed
// dialect.ErrUnsupportedByDialect at Exec time where unsupported. Join
// copies del.joins into a FRESH backing
// array before appending.
func (del Delete[T]) Join(rel joinRelation[T], joinType JoinType) Delete[T] {
	spec := rel.joinSpec(del.table)

	next := make([]render.JoinSpec, 0, len(del.joins)+1)
	next = append(next, del.joins...)
	next = append(next, spec)
	del.joins = next
	del.joinType = joinType

	return del
}

// Where combines p into del's existing filter with AND, matching
// Query[T].Where's rule exactly.
func (del Delete[T]) Where(p Predicate[T]) Delete[T] {
	if del.where.IsSet() {
		del.where = And(del.where, p)
	} else {
		del.where = p
	}

	return del
}

// OrderBy appends terms to del's ORDER BY list, turning the DELETE into
// `DELETE ... ORDER BY ...`. It copies del.order into a FRESH backing
// array before appending, matching Query[T].OrderBy's branch-safety rule.
// The tail is capability-gated at Exec time: no in-tree dialect supports
// it, so on Postgres or SQLite (or on a joined DELETE on a dialect without
// joined support) Exec returns a typed dialect.ErrUnsupportedByDialect
// instead of rendering SQL the engine would reject.
func (del Delete[T]) OrderBy(terms ...OrderTerm[T]) Delete[T] {
	next := make([]OrderTerm[T], 0, len(del.order)+len(terms))
	next = append(next, del.order...)
	next = append(next, terms...)
	del.order = next

	return del
}

// Limit caps how many rows the DELETE removes. A value of 0 (the zero
// value) renders NO LIMIT clause -- unlimited rows -- matching the
// Query[T].Limit zero-value-is-unset convention. On a dialect whose engine
// rejects LIMIT on DELETE, Exec returns a typed
// dialect.ErrUnsupportedByDialect.
func (del Delete[T]) Limit(n int) Delete[T] {
	del.limit = n

	return del
}

// Offset skips the first n rows before the DELETE removes any. A value of
// 0 (the zero value) renders no OFFSET clause, matching Query[T].Offset.
// OFFSET on DELETE is not supported by any in-tree dialect (no in-tree
// engine accepts a DML tail), so Exec returns a typed
// dialect.ErrUnsupportedByDialect whenever n > 0.
func (del Delete[T]) Offset(n int) Delete[T] {
	del.offset = n

	return del
}

// Returning appends a `RETURNING <columns>` clause to del, turning the
// DELETE into one that returns the deleted rows instead of only removing
// them. Returning() with no arguments returns every column of T in codegen
// order, so ExecReturning's positional scan into T lines up exactly; an
// explicit column subset is allowed but is then the caller's responsibility
// to keep aligned with T's codegen'd Scan method.
//
// RETURNING is supported on Postgres and SQLite. On a dialect without
// RETURNING support, ExecReturning fails with a typed
// dialect.ErrUnsupportedByDialect at execution time -- the clause is never
// silently dropped.
func (del Delete[T]) Returning(cols ...AnyColumn[T]) Delete[T] {
	if len(cols) == 0 {
		del.returning = del.table.Columns()

		return del
	}

	cols2 := make([]string, len(cols))
	for j, c := range cols {
		cols2[j] = c.Name()
	}

	del.returning = cols2

	return del
}

// requireStatementCapability checks the per-statement capability del
// needs: a joined DELETE requires dialect.MutateJoinDialect support (see
// requireMutateJoin), and an ORDER BY / LIMIT / OFFSET tail requires
// dialect.MutateOrderDialect support (see requireMutateOrder). A plain
// DELETE needs nothing beyond the base dialect, and RETURNING is checked
// separately by ExecReturning.
func (del Delete[T]) requireStatementCapability(d dialect.Dialect) error {
	if len(del.joins) > 0 {
		if err := requireMutateJoin(d, del.joinType, false); err != nil {
			return err
		}

		return requireMutateOrder(d, len(del.order) > 0, del.limit, del.offset, true, false)
	}

	return requireMutateOrder(d, len(del.order) > 0, del.limit, del.offset, false, false)
}

// renderStatement renders del for d, dispatching the plain (shape-cached
// render.Delete) vs joined (render.DeleteJoin) DELETE syntax and appending
// the ORDER BY / LIMIT / OFFSET tail and the RETURNING clause when one was
// requested. The capability gates -- mutate-join, mutate-order and
// returning -- are checked by the caller before this is reached; this only
// picks the renderer.
func (del Delete[T]) renderStatement(d dialect.Dialect) (query string, args []any, err error) {
	where := toRenderNode[T](del.where.Render())
	order := toRenderOrder(del.order)

	if len(del.joins) > 0 {
		if len(del.returning) > 0 {
			return render.DeleteJoinReturning(d, del.table.Name(), del.joins, del.joinType, where, order, del.limit, del.offset, del.returning)
		}

		return render.DeleteJoin(d, del.table.Name(), del.joins, del.joinType, where, order, del.limit, del.offset)
	}

	if len(del.returning) > 0 {
		return render.DeleteReturning(d, del.table.Name(), where, order, del.limit, del.offset, del.returning)
	}

	return render.Delete(d, del.table.Name(), where, order, del.limit, del.offset)
}

// Exec runs del against exec and returns the number of rows affected. Exec
// returns an error if Returning was requested -- a RETURNING statement
// returns rows and must run through ExecReturning instead, never silently
// dropping the clause.
func (del Delete[T]) Exec(ctx context.Context, exec db.DB) (int64, error) {
	if len(del.returning) > 0 {
		return 0, errors.New("orm: Delete.Exec: RETURNING requested via Returning(); use ExecReturning")
	}

	d, err := resolveDialect(exec)
	if err != nil {
		return 0, err
	}

	err = del.requireStatementCapability(d)
	if err != nil {
		return 0, err
	}

	query, args, err := del.renderStatement(d)
	if err != nil {
		return 0, fmt.Errorf("orm: Delete.Exec: %w", err)
	}

	encoded := encodeArgs(d, args)
	logQuery(query, encoded)

	n, err := execQuery(ctx, exec, query, encoded)
	if err != nil {
		return 0, fmt.Errorf("orm: Delete.Exec: %w", err)
	}

	return n, nil
}

// ExecReturning runs del with its RETURNING clause and returns the deleted
// row(s), scanned via T's codegen'd Scan method -- the same positional
// scan machinery Query.All (and Insert.ExecReturning) uses, through the
// shared execReturning tail. callers write
//
//	rows, err := DeleteFrom(widgets).
//		Where(widgetID.Eq("w1")).
//		Returning().
//		ExecReturning(ctx, conn)
//
// and rows is []*widget carrying each deleted row's final column values.
//
// ExecReturning requires dialect.ReturningDialect.SupportsReturning to
// report true; on a dialect without it, it returns a typed
// dialect.ErrUnsupportedByDialect. A DELETE that matches no rows produces
// no RETURNING rows.
func (del Delete[T]) ExecReturning[PT ptrScanner[T]](ctx context.Context, exec db.DB) ([]PT, error) {
	if len(del.returning) == 0 {
		return nil, errors.New("orm: Delete.ExecReturning: no RETURNING columns; call Returning() first")
	}

	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	err = requireReturning(d)
	if err != nil {
		return nil, err
	}

	err = del.requireStatementCapability(d)
	if err != nil {
		return nil, err
	}

	query, args, err := del.renderStatement(d)
	if err != nil {
		return nil, fmt.Errorf("orm: Delete.ExecReturning: %w", err)
	}

	return execReturning[T, PT](ctx, exec, d, query, args, "Delete.ExecReturning")
}
