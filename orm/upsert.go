package orm

import (
	"context"
	"errors"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// conflictAction distinguishes the two upsert conflict actions.
type conflictAction int

const (
	conflictDoNothing conflictAction = iota
	conflictDoUpdate
)

// conflictSpec is the resolved ON CONFLICT state
// an Insert[T] carries once OnConflict has been applied. A nil *conflictSpec
// means the Insert is a plain INSERT.
type conflictSpec[T any] struct {
	action conflictAction
	target []string
	// targetWhere is the partial-index predicate on the conflict target
	// (`ON CONFLICT (cols) WHERE <pred>`); the zero Node means "no
	// predicate". It is set by Conflict.Where.
	targetWhere Node
	sets        []Assignment[T]
	// updateWhere is the condition on the DO UPDATE SET clause
	// (`DO UPDATE SET ... WHERE <pred>`); the zero Node means "no
	// predicate". It is set by ConflictUpdate.Where.
	updateWhere Node
}

// Conflict is the half-built ON CONFLICT clause Insert[T].OnConflict
// returns: it pins the conflict target columns and, optionally via Where, the
// partial-index target predicate. DoNothing or DoUpdate then picks the
// action and returns the finished Insert. It is a value type, following the
// same copy-on-write discipline as Insert[T] itself.
type Conflict[T any] struct {
	i      Insert[T]
	target []string
	where  Node
}

// ConflictUpdate is the half-built DO UPDATE clause Conflict.DoUpdate
// returns: it carries the finished upsert plus the optional conditional
// update, which Where then attaches. It embeds Insert[T] so every Insert
// method (Exec, ExecReturning, Returning, Select, ...) is directly
// available; Where returns the finished Insert[T]. Like every other builder
// it is a value type with copy-on-write semantics.
type ConflictUpdate[T any] struct {
	Insert[T]
}

// OnConflict starts an upsert: when the insert's target columns collide
// with an existing row's unique constraint, the action chosen by
// DoNothing/DoUpdate runs instead of the insert failing.
//
// On Postgres and SQLite the columns render as the ON CONFLICT target, e.g.
// `ON CONFLICT ("id") DO UPDATE SET ...`. Naming no columns is valid there
// too and means "conflict on any unique constraint".
func (i Insert[T]) OnConflict(cols ...AnyColumn[T]) Conflict[T] {
	target := make([]string, len(cols))
	for j, c := range cols {
		target[j] = c.Name()
	}

	return Conflict[T]{i: i, target: target}
}

// Where attaches the partial-index predicate to the conflict target,
// producing `ON CONFLICT (cols) WHERE <predicate>`. The predicate names the
// partial unique index to arbitrate on: only rows for which it holds are
// candidates for the index's uniqueness. Call it before DoNothing/DoUpdate,
// e.g.
//
//	InsertInto(t).Values(...).
//		OnConflict(id.Col()).Where(active.Eq(true)).
//		DoUpdate(Set(name, "x")).Where(quantity.Lt(100))
//
// The predicate must match the named partial index's own WHERE clause, and
// Postgres and SQLite both compare it structurally at prepare time -- a
// bound parameter cannot stand in for a constant index predicate. Express
// such a schema-level predicate as literal SQL through the audited
// orm.UnsafeRaw escape hatch (e.g. UnsafeRaw[T]("active = 1")), not a
// parameter-binding column comparison; a column comparison still renders
// and binds correctly, but will not identify a partial index.
func (c Conflict[T]) Where(pred Predicate[T]) Conflict[T] {
	c.where = pred.Render()

	return c
}

// DoNothing finishes OnConflict into `ON CONFLICT ... DO NOTHING`: a
// conflicting row is left untouched.
func (c Conflict[T]) DoNothing() Insert[T] {
	c.i.conflict = &conflictSpec[T]{action: conflictDoNothing, target: c.target, targetWhere: c.where}

	return c.i
}

// DoUpdate finishes OnConflict into `ON CONFLICT ... DO UPDATE SET <sets>`,
// applying sets to the conflicting row. sets must be non-empty.
//
// DoUpdate returns a ConflictUpdate[T] stage rather than the finished
// Insert directly, so the optional conditional update predicate can be
// attached unambiguously: `.DoUpdate(sets...).Where(updatePred)`. The stage
// embeds Insert[T], so when no update predicate is needed it can be used
// exactly like the Insert DoUpdate used to return (Exec, Returning,
// ExecReturning, Select, ...).
func (c Conflict[T]) DoUpdate(sets ...Assignment[T]) ConflictUpdate[T] {
	cp := make([]Assignment[T], len(sets))
	copy(cp, sets)

	c.i.conflict = &conflictSpec[T]{action: conflictDoUpdate, target: c.target, targetWhere: c.where, sets: cp}

	return ConflictUpdate[T]{Insert: c.i}
}

// Where attaches a condition to the DO UPDATE SET clause, producing
// `... DO UPDATE SET ... WHERE <predicate>`: the assignments apply only to
// rows for which the predicate holds, and rows that fail it are left
// unchanged. The predicate is evaluated against the existing target row
// (not the proposed/excluded row). Supported on Postgres and SQLite; on a
// dialect without the capability a requested update predicate fails with a
// typed dialect.ErrUnsupportedByDialect at execution time, never silently
// dropped.
func (u ConflictUpdate[T]) Where(pred Predicate[T]) Insert[T] {
	cp := *u.conflict
	cp.updateWhere = pred.Render()
	u.conflict = &cp

	return u.Insert
}

// Returning appends a `RETURNING <columns>` clause, turning the upsert into
// one that returns the inserted/updated row(s) instead of only writing
// them. Returning() with no arguments returns every column of T in codegen
// order, so ExecReturning's positional scan into T lines up exactly; an
// explicit column subset is allowed but is then the caller's responsibility
// to keep aligned with T's codegen'd Scan method.
//
// RETURNING is supported on Postgres and SQLite. On a dialect without the
// capability, ExecReturning fails with a typed
// dialect.ErrUnsupportedByDialect at execution time.
func (i Insert[T]) Returning(cols ...AnyColumn[T]) Insert[T] {
	if len(cols) == 0 {
		i.returning = i.table.Columns()

		return i
	}

	cols2 := make([]string, len(cols))
	for j, c := range cols {
		cols2[j] = c.Name()
	}

	i.returning = cols2

	return i
}

// requireReturning reports whether d can run a RETURNING clause: the
// dialect.ReturningDialect capability must be present and SupportsReturning
// must report true. A missing capability returns a typed
// dialect.ErrUnsupportedByDialect, never a panic or silently rendering
// RETURNING for a dialect that rejects it.
func requireReturning(d dialect.Dialect) error {
	rd, ok := d.(dialect.ReturningDialect)
	if !ok || !rd.SupportsReturning() {
		return fmt.Errorf("orm: %w: dialect %q does not support RETURNING", dialect.ErrUnsupportedByDialect, d.Name())
	}

	return nil
}

// requireConflictWhere reports whether d can render the predicate the upsert
// actually carries. A target predicate needs
// dialect.ConflictWhereDialect.SupportsConflictTargetWhere; an update
// predicate needs SupportsConflictUpdateWhere. A missing capability -- a
// base-only dialect, or one that implements neither -- returns a typed
// dialect.ErrUnsupportedByDialect, never a silently-dropped predicate. An
// upsert with no predicate is never gated.
func (i Insert[T]) requireConflictWhere(d dialect.Dialect) error {
	if i.conflict.targetWhere.Kind != NNone {
		if err := requireConflictWhereCapability(d, true); err != nil {
			return err
		}
	}

	if i.conflict.updateWhere.Kind != NNone {
		if err := requireConflictWhereCapability(d, false); err != nil {
			return err
		}
	}

	return nil
}

// requireConflictWhereCapability is the shared ConflictWhereDialect check:
// target selects which of the two explicit capability booleans gates the
// predicate.
func requireConflictWhereCapability(d dialect.Dialect, target bool) error {
	cd, ok := d.(dialect.ConflictWhereDialect)

	feature := "a DO UPDATE WHERE predicate"
	supported := ok && cd.SupportsConflictUpdateWhere()

	if target {
		feature = "an ON CONFLICT target WHERE predicate"
		supported = ok && cd.SupportsConflictTargetWhere()
	}

	if !supported {
		return fmt.Errorf("orm: %w: dialect %q does not support %s", dialect.ErrUnsupportedByDialect, d.Name(), feature)
	}

	return nil
}

// requireDefaultValuesExclusive fails closed when an explicit DefaultValues
// insert also carries another row source (Values rows, an INSERT ... SELECT
// source, or an explicit Columns list) -- those are mutually exclusive, and
// silently picking one would be the kind of wrong-SQL fallback the
// capability gates forbid. It is a no-op for a non-DefaultValues insert.
func (i Insert[T]) requireDefaultValuesExclusive() error {
	if !i.defaultValues {
		return nil
	}

	switch {
	case i.rowsLen > 0:
		return errors.New("orm: Insert.DefaultValues: mutually exclusive with Values rows")
	case i.selectSrc != nil:
		return errors.New("orm: Insert.DefaultValues: mutually exclusive with Select")
	case len(i.columns) > 0:
		return errors.New("orm: Insert.DefaultValues: mutually exclusive with Columns")
	}

	return nil
}

// renderInsert renders i for d, dispatching the ON CONFLICT (Postgres/
// SQLite) conflict syntax. Argument order
// is always values-then-conflict-assignments, with the renderer's
// argCounter continuing across both so Postgres placeholders number
// correctly.
func (i Insert[T]) renderInsert(d dialect.Dialect) (query string, args []any, err error) {
	// DefaultValues is an explicit "no source" declaration, so any other row
	// source on the same chain is a mutual-exclusion error rather than a
	// silently-picked source -- checked before the SELECT dispatch so a
	// DefaultValues+Select chain can never render the SELECT instead.
	err = i.requireDefaultValuesExclusive()
	if err != nil {
		return "", nil, err
	}

	if i.selectSrc != nil {
		return i.renderInsertSelect(d)
	}

	if len(i.columns) == 0 && i.rowsLen > 0 {
		return "", nil, fmt.Errorf(
			"orm: Insert: %d row(s) have no columns (a first Values() call with no assignments "+
				"fixes an empty column list; it cannot precede real rows)",
			i.rowsLen,
		)
	}

	if i.conflict == nil {
		switch i.rowsLen {
		case 0:
			q, a, insertErr := render.InsertReturning(d, i.table.Name(), nil, nil, i.returning)

			return q, a, insertErr
		case 1:
			q, a, insertErr := render.InsertReturning(d, i.table.Name(), i.columns, i.rows.values, i.returning)

			return q, a, insertErr
		default:
			q, a, insertErr := render.InsertManyReturning(d, i.table.Name(), i.columns, i.rowsSlice(), i.returning)

			return q, a, insertErr
		}
	}

	if i.rowsLen == 0 {
		return "", nil, errors.New("orm: upsert requires at least one Values row (ON CONFLICT cannot combine with DEFAULT VALUES)")
	}

	if i.conflict.action == conflictDoUpdate && len(i.conflict.sets) == 0 {
		return "", nil, errors.New("orm: upsert DO UPDATE requires at least one Set assignment")
	}

	// A partial-index target predicate or conditional-update predicate is
	// gated here, before either dialect's renderer is reached, so an
	// unsupported predicate is a typed dialect.ErrUnsupportedByDialect --
	// never silently dropped.
	err = i.requireConflictWhere(d)
	if err != nil {
		return "", nil, err
	}

	// The conflict clause renders ON CONFLICT (Postgres/SQLite) with the
	// target, predicates and SET assignments resolved above.

	var sets []render.Assignment
	if i.conflict.action == conflictDoUpdate {
		sets = toRenderAssignments(i.conflict.sets)
	}

	where := render.ConflictWhere{
		Target: toRenderNode[T](i.conflict.targetWhere),
		Update: toRenderNode[T](i.conflict.updateWhere),
	}

	if i.rowsLen == 1 {
		q, a, insertErr := render.InsertOnConflictWhere(d, i.table.Name(), i.columns, i.rows.values, i.conflict.target, where, sets, i.returning)
		if insertErr != nil {
			return "", nil, insertErr
		}

		return q, a, nil
	}

	q, a, insertErr := render.InsertManyOnConflictWhere(d, i.table.Name(), i.columns, i.rowsSlice(), i.conflict.target, where, sets, i.returning)
	if insertErr != nil {
		return "", nil, insertErr
	}

	return q, a, nil
}

// ExecReturning runs the upsert with its RETURNING clause and returns the
// upserted row(s), scanned via T's codegen'd Scan method -- the same
// positional scan machinery Query.All uses, so ExecReturning reuses the
// ptrScanner[T] pattern. It executes through exec.Query rather than
// exec.Exec because a RETURNING statement returns rows. PT is inferred from
// T at the call site (the curiously-recurring-generic trick the Query
// builder documents), so callers write
//
//	rows, err := InsertInto(widgets).Values(...).Returning().ExecReturning(ctx, conn)
//
// and rows is []*widget.
//
// ExecReturning requires dialect.ReturningDialect.SupportsReturning to
// report true; on a dialect without it, it returns a typed
// dialect.ErrUnsupportedByDialect. For an ON CONFLICT DO NOTHING upsert, a
// row that hit the conflict produces no RETURNING row, so the result may be
// shorter than the number of rows supplied to Values.
func (i Insert[T]) ExecReturning[PT ptrScanner[T]](ctx context.Context, exec db.DB) ([]PT, error) {
	if len(i.returning) == 0 {
		return nil, errors.New("orm: Insert.ExecReturning: no RETURNING columns; call Returning() first")
	}

	d, err := resolveDialect(exec)
	if err != nil {
		return nil, err
	}

	err = requireReturning(d)
	if err != nil {
		return nil, err
	}

	query, args, err := i.renderInsert(d)
	if err != nil {
		return nil, err
	}

	return execReturning[T, PT](ctx, exec, d, query, args, "Insert.ExecReturning")
}

// execReturning runs an already-rendered RETURNING statement against exec
// and scans every returned row into T via PT -- the same positional scan
// machinery Query.All uses (the ptrScanner[T] pattern needs no reflection). It is the shared execution tail behind
// Insert/Update/Delete ExecReturning; tag names the caller for error
// wrapping. A RETURNING statement returns rows, so it executes through
// queryRows rather than execQuery. The dialect's RETURNING capability must
// already have been checked by the caller; this only runs and scans.
func execReturning[T any, PT ptrScanner[T]](ctx context.Context, exec db.DB, d dialect.Dialect, query string, args []any, tag string) ([]PT, error) {
	encoded := encodeArgs(d, args)
	logQuery(query, encoded)

	rows, err := queryRows(ctx, exec, query, encoded)
	if err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}
	defer func() { _ = rows.Close() }()

	var out []PT

	for rows.Next() {
		var v T

		p := PT(&v)
		if err := p.Scan(rows); err != nil {
			return nil, fmt.Errorf("orm: %s: scan: %w", tag, err)
		}

		out = append(out, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}

	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("orm: %s: %w", tag, err)
	}

	return out, nil
}
