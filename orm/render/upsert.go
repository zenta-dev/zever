package render

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// ConflictWhere carries the two optional WHERE predicates of an upsert. A
// zero Node (Kind == KindNone) means "no predicate" and renders nothing:
//
//   - Target is the partial-index predicate appended to the conflict target
//     (`ON CONFLICT (cols) WHERE <Target>`), identifying the partial unique
//     index to arbitrate on. It is supported by Postgres and SQLite.
//   - Update is the condition appended to the DO UPDATE SET clause
//     (`... DO UPDATE SET ... WHERE <Update>`), under which the update
//     applies only to rows the predicate holds for. It is supported by
//     Postgres and SQLite.
//
// Neither predicate binds before the row/SELECT values; Target's arguments
// are bound immediately after them (matching text order, before the SET
// list) and Update's after the SET list. The orm layer rejects a non-zero
// predicate on a dialect whose ConflictWhereDialect reports the form
// unsupported, so this struct is only ever rendered for a dialect that
// accepts it.
type ConflictWhere struct {
	Target Node
	Update Node
}

// writeInsertValues writes the base `INSERT INTO <table> (<cols>) VALUES
// (<...>), (<...>), ...` clause for rows, advancing counter for every
// placeholder it emits and appending each value to args in placeholder
// order. It is the shared head of the upsert renderers below; callers are
// responsible for ensuring rows is non-empty (an upsert cannot combine with
// DEFAULT VALUES).
func writeInsertValues(b *strings.Builder, d dialect.Dialect, table string, columns []string, rows [][]any, counter *argCounter, args *[]any) {
	n := len(columns)

	if len(rows) > 0 && len(rows[0]) < n {
		n = len(rows[0])
	}

	quoted := make([]string, n)

	for i := 0; i < n; i++ {
		quoted[i] = d.QuoteIdent(columns[i])
	}

	b.WriteString("INSERT INTO ")
	b.WriteString(d.QuoteIdent(table))
	b.WriteString(" (")
	b.WriteString(strings.Join(quoted, ", "))
	b.WriteString(") VALUES ")

	for i, row := range rows {
		if i > 0 {
			b.WriteString(", ")
		}

		rowN := n
		if len(row) < rowN {
			rowN = len(row)
		}

		placeholders := make([]string, rowN)
		for j := 0; j < rowN; j++ {
			placeholders[j] = d.Placeholder(counter.next())
			*args = append(*args, row[j])
		}

		b.WriteString("(")
		b.WriteString(strings.Join(placeholders, ", "))
		b.WriteString(")")
	}
}

// writeOnConflict writes the Postgres/SQLite
// `ON CONFLICT [(target)] [WHERE <target-pred>] DO UPDATE SET ...
// [WHERE <update-pred>] | DO NOTHING` clause. A nil sets renders DO NOTHING;
// a non-nil (non-empty) sets renders DO UPDATE SET with each assignment's
// value bound after the insert values, in order. The optional target
// predicate is rendered (and its arguments bound) before the SET list, and
// the optional update predicate after it, matching their text order; err is
// a rendering error in either predicate (e.g. an unsupported node kind).
func writeOnConflict(b *strings.Builder, d dialect.Dialect, target []string, where ConflictWhere, sets []Assignment, counter *argCounter, args *[]any) error {
	b.WriteString(" ON CONFLICT")

	if len(target) > 0 {
		quoted := make([]string, len(target))
		for i, t := range target {
			quoted[i] = d.QuoteIdent(t)
		}

		b.WriteString(" (")
		b.WriteString(strings.Join(quoted, ", "))
		b.WriteString(")")
	}

	if where.Target.Kind != KindNone {
		clause, wargs, err := renderExpr(d, where.Target, counter)
		if err != nil {
			return fmt.Errorf("orm/render: ON CONFLICT target WHERE: %w", err)
		}

		b.WriteString(" WHERE ")
		b.WriteString(clause)

		*args = append(*args, wargs...)
	}

	if sets == nil {
		b.WriteString(" DO NOTHING")

		return nil
	}

	b.WriteString(" DO UPDATE SET ")

	assignments := make([]string, len(sets))
	for i, s := range sets {
		assignments[i] = quoteColumn(d, s.Column) + " = " + d.Placeholder(counter.next())
		*args = append(*args, s.Value)
	}

	b.WriteString(strings.Join(assignments, ", "))

	if where.Update.Kind != KindNone {
		clause, wargs, err := renderExpr(d, where.Update, counter)
		if err != nil {
			return fmt.Errorf("orm/render: DO UPDATE WHERE: %w", err)
		}

		b.WriteString(" WHERE ")
		b.WriteString(clause)

		*args = append(*args, wargs...)
	}

	return nil
}

// writeReturning writes a `RETURNING <cols>` clause, or nothing when cols
// is empty. RETURNING introduces no bound arguments, so it does not touch
// the counter.
func writeReturning(b *strings.Builder, d dialect.Dialect, cols []string) {
	if len(cols) == 0 {
		return
	}

	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = d.QuoteIdent(c)
	}

	b.WriteString(" RETURNING ")
	b.WriteString(strings.Join(quoted, ", "))
}

// InsertOnConflict renders a single-row
// `INSERT ... ON CONFLICT [target] DO UPDATE SET ... | DO NOTHING
// [RETURNING ...]` for a dialect that supports ON CONFLICT (Postgres,
// SQLite). A nil sets renders DO NOTHING; a non-nil sets renders DO UPDATE
// SET, with every assignment's value bound AFTER the row's insert values in
// the argument list (placeholder numbering continues across the two, so a
// Postgres dialect gets $1..$N for the row and then $N+1.. for the
// assignments). returning is rendered only if non-empty.
//
// It is the no-predicate projection of InsertOnConflictWhere and delegates
// to it, so the two can never render differently.
func InsertOnConflict(d dialect.Dialect, table string, columns []string, values []any, target []string, sets []Assignment, returning []string) (query string, args []any) {
	query, args, _ = InsertOnConflictWhere(d, table, columns, values, target, ConflictWhere{}, sets, returning)

	return query, args
}

// InsertOnConflictWhere is InsertOnConflict with the two optional upsert
// predicates: where.Target renders `ON CONFLICT (cols) WHERE <predicate>`
// after the target (its arguments bound before the SET list) and
// where.Update renders `... DO UPDATE SET ... WHERE <predicate>` after the
// assignments (its arguments bound last). err is non-nil when either
// predicate fails to render; a zero predicate renders nothing.
func InsertOnConflictWhere(
	d dialect.Dialect, table string, columns []string, values []any, target []string, where ConflictWhere, sets []Assignment, returning []string,
) (query string, args []any, err error) {
	var b strings.Builder

	counter := &argCounter{}

	writeInsertValues(&b, d, table, columns, [][]any{values}, counter, &args)

	if err := writeOnConflict(&b, d, target, where, sets, counter, &args); err != nil {
		return "", nil, err
	}

	writeReturning(&b, d, returning)

	return b.String(), args, nil
}

// InsertManyOnConflict is InsertOnConflict's multi-row variant, rendering
// one VALUES tuple per row. The conflict/returning clauses are shared
// across all rows, and args are every row's values flattened in row-major
// order followed by the DO UPDATE SET assignments. It is the no-predicate
// projection of InsertManyOnConflictWhere and delegates to it.
func InsertManyOnConflict(d dialect.Dialect, table string, columns []string, rows [][]any, target []string, sets []Assignment, returning []string) (query string, args []any) {
	query, args, _ = InsertManyOnConflictWhere(d, table, columns, rows, target, ConflictWhere{}, sets, returning)

	return query, args
}

// InsertManyOnConflictWhere is InsertManyOnConflict with the two optional
// upsert predicates (see InsertOnConflictWhere): all row values are bound
// first, then where.Target's arguments, then the SET assignments, then
// where.Update's arguments -- matching the rendered text order.
func InsertManyOnConflictWhere(
	d dialect.Dialect, table string, columns []string, rows [][]any, target []string, where ConflictWhere, sets []Assignment, returning []string,
) (query string, args []any, err error) {
	var b strings.Builder

	counter := &argCounter{}

	writeInsertValues(&b, d, table, columns, rows, counter, &args)

	if err := writeOnConflict(&b, d, target, where, sets, counter, &args); err != nil {
		return "", nil, err
	}

	writeReturning(&b, d, returning)

	return b.String(), args, nil
}
