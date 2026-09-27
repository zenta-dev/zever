package orm

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/render"
)

// MergeAssignment is one target-column assignment in a MERGE clause, built
// by MergeSource (assign from the source row) or MergeValue (assign a
// literal). Its fields are unexported so an assignment always carries a real
// target column name, matching orm's identifier-safety rule.
type MergeAssignment struct {
	column string
	source string
	value  any
}

// MergeSource builds a MERGE assignment that copies the source row's column
// source into the target column target, rendered `target = source.source`.
// The two columns share value type V, so a mismatched pair does not compile.
func MergeSource[T any, U any, V any](target Column[T, V], source Column[U, V]) MergeAssignment {
	return MergeAssignment{column: target.Name(), source: source.Name()}
}

// MergeValue builds a MERGE assignment that sets the target column to a
// literal value, bound as a placeholder. target may be any column of the
// target entity (including a nullable one, via .Col()).
func MergeValue[T any](target AnyColumn[T], value any) MergeAssignment {
	return MergeAssignment{column: target.Name(), value: value}
}

// MergeTarget is the half-built MERGE stage MergeInto returns: it pins the
// target entity and waits for the source. UsingSource completes it into a
// Merge.
type MergeTarget[T any] struct {
	table Table[T]
}

// MergeInto starts a MERGE against target: the SQL-standard upsert-like
// statement `MERGE INTO target USING source ON ... WHEN MATCHED THEN ...
// WHEN NOT MATCHED THEN ...`. It is Postgres 15+ only; SQLite has
// no MERGE, so Exec returns a typed dialect.ErrUnsupportedByDialect there
// (see dialect.MergeDialect). The target and source entities may differ.
func MergeInto[T any](t Table[T]) MergeTarget[T] {
	return MergeTarget[T]{table: t}
}

// UsingSource completes the MERGE with its source table, returning a Merge
// over the two entities.
func (m MergeTarget[T]) UsingSource[U any](source Table[U]) Merge[T, U] {
	return Merge[T, U]{target: m.table, source: source}
}

// Merge is an immutable, value-type SQL MERGE builder over a target entity T
// and a source entity U, following the same copy-on-write chain discipline
// as Query[T]/Insert[T]: every chain method returns a NEW Merge and copies
// the slice it appends to, so branching a chain is safe by construction.
//
// MERGE is the SQL-standard conditional write: rows matching the ON
// condition are UPDATEd or DELETEd by a WHEN MATCHED clause, and rows with no
// match are INSERTed by a WHEN NOT MATCHED clause. It is supported by
// Postgres 15+; SQLite rejects it with a typed
// dialect.ErrUnsupportedByDialect at Exec time. When clauses are optional
// individually but at least one is required to render.
type Merge[T any, U any] struct {
	target Table[T]
	source Table[U]
	on     []render.JoinSpec
	whens  []render.MergeWhen
}

// On appends an equi-join condition between a target column and a source
// column, rendered `target.col = source.col`. Multiple On calls AND their
// conditions together; at least one is required to Exec. On copies the
// backing slice before appending, matching the branch-safety rule of every
// other chain method.
func (m Merge[T, U]) On(targetCol AnyColumn[T], sourceCol AnyColumn[U]) Merge[T, U] {
	next := make([]render.JoinSpec, 0, len(m.on)+1)
	next = append(next, m.on...)
	next = append(next, render.JoinSpec{
		ParentCol:  targetCol.Name(),
		ChildTable: m.source.Name(),
		ChildCol:   sourceCol.Name(),
	})
	m.on = next

	return m
}

// WhenMatchedUpdate appends a `WHEN MATCHED THEN UPDATE SET <sets>` clause.
// Each set is a MergeSource/MergeValue assignment; at least one is required.
func (m Merge[T, U]) WhenMatchedUpdate(sets ...MergeAssignment) Merge[T, U] {
	return m.appendWhen(render.MergeWhen{
		Matched: true,
		Action:  render.MergeUpdate,
		Sets:    toRenderMergeAssignments(sets),
	})
}

// WhenMatchedDelete appends a `WHEN MATCHED THEN DELETE` clause.
func (m Merge[T, U]) WhenMatchedDelete() Merge[T, U] {
	return m.appendWhen(render.MergeWhen{Matched: true, Action: render.MergeDelete})
}

// WhenNotMatchedInsert appends a
// `WHEN NOT MATCHED THEN INSERT (<cols>) VALUES (<vals>)` clause. With no
// assignments it renders `INSERT DEFAULT VALUES`; otherwise the target column
// list and the value list are taken in assignment order (each value a source
// column or a literal).
func (m Merge[T, U]) WhenNotMatchedInsert(sets ...MergeAssignment) Merge[T, U] {
	return m.appendWhen(render.MergeWhen{
		Matched: false,
		Action:  render.MergeInsert,
		Sets:    toRenderMergeAssignments(sets),
	})
}

// appendWhen is the shared branch-safe WHEN-clause append.
func (m Merge[T, U]) appendWhen(w render.MergeWhen) Merge[T, U] {
	next := make([]render.MergeWhen, 0, len(m.whens)+1)
	next = append(next, m.whens...)
	next = append(next, w)
	m.whens = next

	return m
}

// toRenderMergeAssignments erases the typed assignments into render's shape,
// mirroring toRenderAssignments.
func toRenderMergeAssignments(sets []MergeAssignment) []render.MergeAssignment {
	out := make([]render.MergeAssignment, len(sets))

	for i, s := range sets {
		out[i] = render.MergeAssignment{Column: s.column, Source: s.source, Value: s.value}
	}

	return out
}

// Exec runs m against exec and returns the number of rows affected. MERGE is
// supported on Postgres 15+ only: any other dialect returns a typed
// dialect.ErrUnsupportedByDialect before any SQL is issued. A malformed MERGE
// (no ON condition, no WHEN clause, an invalid arm/action pairing) is a
// render-time error, never silently-broken SQL.
func (m Merge[T, U]) Exec(ctx context.Context, exec db.DB) (int64, error) {
	d, err := resolveDialect(exec)
	if err != nil {
		return 0, err
	}

	md, ok := d.(dialect.MergeDialect)
	if !ok || !md.SupportsMerge() {
		return 0, fmt.Errorf("orm: %w: dialect %q does not support MERGE", dialect.ErrUnsupportedByDialect, d.Name())
	}

	query, args, err := render.Merge(d, m.target.Name(), m.source.Name(), m.on, m.whens)
	if err != nil {
		return 0, fmt.Errorf("orm: Merge.Exec: %w", err)
	}

	encoded := encodeArgs(d, args)
	logQuery(query, encoded)

	n, err := execQuery(ctx, exec, query, encoded)
	if err != nil {
		return 0, fmt.Errorf("orm: Merge.Exec: %w", err)
	}

	return n, nil
}
