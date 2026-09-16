package render

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// Assignment is one column=value pair for an UPDATE statement's SET
// clause. It is render's own copy of the builder's Assignment erased shape;
// the builder's Assignment values are converted to this shape with a plain
// field-for-field literal at the call site (see the mutation builder), keeping
// render decoupled from the orm root package (no import cycle) -- the same
// pattern the query builder's node/order converters already use for
// Node/OrderTerm.
type Assignment struct {
	Column string
	Value  any
}

// renderInsertText renders `INSERT INTO <table> (<columns>) VALUES (<...> )`
// and its positional arguments for dialect. It is the cache-miss body
// behind Insert (see shapecache.go). columns and values are positional:
// values[i] is the value for columns[i]. A mismatched length is not an
// error here -- the shorter of the two wins, so a caller can never produce
// a statement whose placeholder count disagrees with its argument count.
// Ported from orm/engine.RenderInsert, adapted to placeholder numbering
// via argCounter (so a Postgres-shaped dialect gets $1, $2, ... rather
// than a bare loop index) matching render's existing Select/Count
// convention. It is infallible by construction (identifier quoting and
// placeholder rendering never fail), so it returns no error.
func renderInsertText(d dialect.Dialect, table string, columns []string, values []any) (query string, args []any) {
	n := len(columns)
	if len(values) < n {
		n = len(values)
	}

	var b strings.Builder

	b.WriteString("INSERT INTO ")
	b.WriteString(d.QuoteIdent(table))

	if n == 0 {
		b.WriteString(" DEFAULT VALUES")

		return b.String(), nil
	}

	counter := &argCounter{}
	quoted := make([]string, n)
	placeholders := make([]string, n)
	args = make([]any, n)

	for i := 0; i < n; i++ {
		quoted[i] = d.QuoteIdent(columns[i])
		placeholders[i] = d.Placeholder(counter.next())
		args[i] = values[i]
	}

	b.WriteString(" (")
	b.WriteString(strings.Join(quoted, ", "))
	b.WriteString(") VALUES (")
	b.WriteString(strings.Join(placeholders, ", "))
	b.WriteString(")")

	return b.String(), args
}

// renderInsertManyText renders a single multi-row
// `INSERT INTO <table> (<columns>) VALUES (<...>), (<...>), ...` and its
// positional arguments. It is the cache-miss body behind InsertMany (see
// shapecache.go). rows[i] must have the same length as columns --
// unlike Insert this is not forgiving of a mismatch, since a per-row short
// read would silently shift every following row's columns. args is every
// row's values flattened in declaration order, matching the placeholders'
// left-to-right, row-by-row numbering. Ported from
// orm/engine.RenderInsertMany.
//
// This is pure text-rendering with no chunking: the caller (Insert[T].Exec)
// is responsible for keeping the row count within a safe parameter-count
// budget for the target dialect.
func renderInsertManyText(d dialect.Dialect, table string, columns []string, rows [][]any) (query string, args []any, err error) {
	n := len(columns)

	var b strings.Builder

	b.WriteString("INSERT INTO ")
	b.WriteString(d.QuoteIdent(table))

	if n == 0 || len(rows) == 0 {
		b.WriteString(" DEFAULT VALUES")

		return b.String(), nil, nil
	}

	for i, row := range rows {
		if len(row) != n {
			return "", nil, fmt.Errorf("orm/render: InsertMany: row %d has %d values, want %d (one per column)", i, len(row), n)
		}
	}

	quoted := make([]string, n)
	for i := 0; i < n; i++ {
		quoted[i] = d.QuoteIdent(columns[i])
	}

	b.WriteString(" (")
	b.WriteString(strings.Join(quoted, ", "))
	b.WriteString(") VALUES ")

	args = make([]any, 0, n*len(rows))
	counter := &argCounter{}

	for i, row := range rows {
		if i > 0 {
			b.WriteString(", ")
		}

		placeholders := make([]string, n)
		for j := 0; j < n; j++ {
			placeholders[j] = d.Placeholder(counter.next())
			args = append(args, row[j])
		}

		b.WriteString("(")
		b.WriteString(strings.Join(placeholders, ", "))
		b.WriteString(")")
	}

	return b.String(), args, nil
}

// InsertReturning renders Insert followed by a `RETURNING <cols>` clause,
// for a plain (non-conflict) INSERT that must hand the inserted row(s)
// back to the caller. It is Insert's returning-aware twin -- the same
// statement text plus the clause writeReturning appends for the
// ON CONFLICT path, so a plain INSERT's RETURNING is never silently
// dropped (the quirk that originally hid it: render.Insert had no
// returning parameter at all). returning columns are rendered only when
// non-empty; args are unchanged by the clause since RETURNING binds
// nothing.
func InsertReturning(d dialect.Dialect, table string, columns []string, values []any, returning []string) (query string, args []any, err error) {
	// Insert is infallible by construction (renderInsertText returns no
	// error and the shape cache never fails), so there is no error to
	// propagate here; the named err stays nil.
	q, a, _ := Insert(d, table, columns, values)

	if len(returning) == 0 {
		return q, a, nil
	}

	var b strings.Builder

	b.WriteString(q)
	writeReturning(&b, d, returning)

	return b.String(), a, nil
}

// InsertManyReturning is InsertMany's returning-aware twin; see
// InsertReturning for the reasoning.
func InsertManyReturning(d dialect.Dialect, table string, columns []string, rows [][]any, returning []string) (query string, args []any, err error) {
	q, a, err := InsertMany(d, table, columns, rows)
	if err != nil {
		return "", nil, err
	}

	if len(returning) == 0 {
		return q, a, nil
	}

	var b strings.Builder

	b.WriteString(q)
	writeReturning(&b, d, returning)

	return b.String(), a, nil
}

// renderUpdateText renders `UPDATE <table> SET ... [WHERE ...] [ORDER BY
// ...] [LIMIT ...] [OFFSET ...]` and its positional arguments. It is the
// cache-miss body behind Update (see shapecache.go). sets is iterated in
// caller-declaration order (a []Assignment, not a map), so the rendered
// statement and its arguments are deterministic without needing to sort
// column names. order terms render unqualified; limit/offset bind as
// placeholders only when > 0, matching Select's zero-value-is-unset rule.
// err is non-nil when where contains an unsupported NodeKind/Op. Ported
// from orm/engine.RenderUpdate.
func renderUpdateText(d dialect.Dialect, table string, sets []Assignment, where Node, order []OrderTerm, limit, offset int) (query string, args []any, err error) {
	counter := &argCounter{}

	var b strings.Builder

	b.WriteString("UPDATE ")
	b.WriteString(d.QuoteIdent(table))
	writeSet(&b, d, sets, counter, &args)

	if err := writeWhereClause(&b, d, where, counter, &args); err != nil {
		return "", nil, err
	}

	if err := writeTail(&b, d, order, limit, offset, counter, &args); err != nil {
		return "", nil, err
	}

	return b.String(), args, nil
}

// writeSet writes a ` SET col = ?, ...` clause for sets, advancing counter
// for each placeholder and appending every value to args in declaration
// order. It is the shared SET writer for Update and UpdateJoin.
func writeSet(b *strings.Builder, d dialect.Dialect, sets []Assignment, counter *argCounter, args *[]any) {
	assignments := make([]string, 0, len(sets))

	for _, s := range sets {
		assignments = append(assignments, quoteColumn(d, s.Column)+" = "+d.Placeholder(counter.next()))
		*args = append(*args, s.Value)
	}

	b.WriteString(" SET ")
	b.WriteString(strings.Join(assignments, ", "))
}

// writeWhereClause writes a ` WHERE <clause>` for where, or nothing when
// where is unset. Args are appended in placeholder order.
func writeWhereClause(b *strings.Builder, d dialect.Dialect, where Node, counter *argCounter, args *[]any) error {
	clause, whereArgs, err := renderExpr(d, where, counter)
	if err != nil {
		return err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	*args = append(*args, whereArgs...)

	return nil
}

// writeTail renders the trailing clauses an UPDATE/DELETE shares with a
// SELECT -- `[ORDER BY ...] [LIMIT ...] [OFFSET ...]` -- directly after the
// WHERE clause, numbering placeholders with counter and appending every
// bound value to args. limit/offset render as placeholders only when > 0
// (zero means unset, matching Select). Order terms reuse joinOrderBy, so an
// FTS ranking term or a scalar-expression term carries its bound query
// arguments here too. It is the SELECT-order/limit/offset tail factored out
// of writeSelectTail for the mutation renderers, whose WHERE rendering
// differs (join conditions, table-qualified leaves). err is non-nil when an
// order term's scalar expression is malformed.
func writeTail(b *strings.Builder, d dialect.Dialect, order []OrderTerm, limit, offset int, counter *argCounter, args *[]any) error {
	if len(order) > 0 {
		b.WriteString(" ORDER BY ")

		orderText, orderArgs, err := joinOrderBy(d, order, counter)
		if err != nil {
			return err
		}

		b.WriteString(orderText)

		*args = append(*args, orderArgs...)
	}

	if limit > 0 {
		b.WriteString(" LIMIT ")
		b.WriteString(d.Placeholder(counter.next()))

		*args = append(*args, limit)
	}

	if offset > 0 {
		b.WriteString(" OFFSET ")
		b.WriteString(d.Placeholder(counter.next()))

		*args = append(*args, offset)
	}

	return nil
}

// renderDeleteText renders `DELETE FROM <table> [WHERE ...] [ORDER BY ...]
// [LIMIT ...] [OFFSET ...]` and its positional arguments. It is the
// cache-miss body behind Delete (see shapecache.go). An unset where deletes
// every row; that is the caller's decision to make, not the renderer's
// (mirrors orm/engine.RenderDelete's documented behavior). limit/offset
// bind as placeholders only when > 0. err is non-nil when where contains an
// unsupported NodeKind/Op.
func renderDeleteText(d dialect.Dialect, table string, where Node, order []OrderTerm, limit, offset int) (query string, args []any, err error) {
	var b strings.Builder

	b.WriteString("DELETE FROM ")
	b.WriteString(d.QuoteIdent(table))

	counter := &argCounter{}

	clause, whereArgs, err := renderExpr(d, where, counter)
	if err != nil {
		return "", nil, err
	}

	if clause != "" {
		b.WriteString(" WHERE ")
		b.WriteString(clause)
	}

	args = whereArgs
	if err := writeTail(&b, d, order, limit, offset, counter, &args); err != nil {
		return "", nil, err
	}

	return b.String(), args, nil
}

// UpdateReturning renders Update followed by an optional
// `RETURNING <cols>` clause, for an UPDATE that must hand the updated
// rows back to the caller. It is Update's returning-aware twin: the same
// statement text (shape-cache included) plus the clause writeReturning
// appends, so an UPDATE's RETURNING is never silently dropped. returning
// columns are rendered only when non-empty; args are unchanged by the
// clause since RETURNING binds nothing. RETURNING always comes last,
// after the ORDER BY / LIMIT / OFFSET tail.
func UpdateReturning(
	d dialect.Dialect, table string, sets []Assignment, where Node, order []OrderTerm, limit, offset int, returning []string,
) (query string, args []any, err error) {
	q, a, err := Update(d, table, sets, where, order, limit, offset)
	if err != nil {
		return "", nil, err
	}

	if len(returning) == 0 {
		return q, a, nil
	}

	var b strings.Builder

	b.WriteString(q)
	writeReturning(&b, d, returning)

	return b.String(), a, nil
}

// DeleteReturning renders Delete followed by an optional
// `RETURNING <cols>` clause, for a DELETE that must hand the deleted rows
// back to the caller. It is Delete's returning-aware twin; see
// UpdateReturning for the reasoning.
func DeleteReturning(d dialect.Dialect, table string, where Node, order []OrderTerm, limit, offset int, returning []string) (query string, args []any, err error) {
	q, a, err := Delete(d, table, where, order, limit, offset)
	if err != nil {
		return "", nil, err
	}

	if len(returning) == 0 {
		return q, a, nil
	}

	var b strings.Builder

	b.WriteString(q)
	writeReturning(&b, d, returning)

	return b.String(), a, nil
}

// JoinSpec is render's erased view of a the builder's Relation: the
// two sides of one single-column equi-join (ParentCol on the updated/
// deleted table, ChildCol on ChildTable). The builder converts a Relation to this
// shape at the call site -- render cannot import the orm root without a cycle --
// mirroring how the builder's Assignment is erased to render.Assignment.
type JoinSpec struct {
	ParentCol  string
	ChildTable string
	ChildCol   string
}

// joinConditions renders the equi-join predicate(s) for joins against
// table: "table.ParentCol = ChildTable.ChildCol" per join, ANDed together
// in declaration order. It binds no arguments.
func joinConditions(d dialect.Dialect, table string, joins []JoinSpec) []string {
	out := make([]string, len(joins))

	for i, j := range joins {
		out[i] = quoteColumn(d, table+"."+j.ParentCol) + " = " + quoteColumn(d, j.ChildTable+"."+j.ChildCol)
	}

	return out
}

// writeFromTables writes a comma-separated list of joined child tables,
// the body of the Postgres/SQLite `FROM <tables>` / `USING <tables>`
// clause.
func writeFromTables(b *strings.Builder, d dialect.Dialect, joins []JoinSpec) {
	tables := make([]string, len(joins))

	for i, j := range joins {
		tables[i] = d.QuoteIdent(j.ChildTable)
	}

	b.WriteString(strings.Join(tables, ", "))
}

// writeJoinWhere writes the WHERE clause of a joined UPDATE/DELETE: the join
// conditions ANDed with the caller's where (whose leaves are qualified to
// table, falling back on each leaf's own Table).
func writeJoinWhere(b *strings.Builder, d dialect.Dialect, table string, joins []JoinSpec, where Node, counter *argCounter, args *[]any) error {
	conditions := joinConditions(d, table, joins)

	clause, whereArgs, err := renderExpr(d, qualifyNode(where, table), counter)
	if err != nil {
		return err
	}

	if clause != "" {
		conditions = append(conditions, clause)
	}

	if len(conditions) > 0 {
		b.WriteString(" WHERE ")
		b.WriteString(strings.Join(conditions, " AND "))
	}

	*args = append(*args, whereArgs...)

	return nil
}

// UpdateJoin renders an UPDATE against one or more joined tables:
// `UPDATE <table> SET ... FROM <joined> WHERE ...` on Postgres and SQLite.
// The join conditions always render (a joined UPDATE without them would be a
// cross update); the caller's where renders after them, ANDed. An ORDER BY
// / LIMIT / OFFSET tail renders after the WHERE. set values are bound
// first, then where values, in placeholder order -- matching the plain
// Update convention. A dialect that supports neither syntax must be
// rejected by the caller before this renderer is reached (the
// MutateJoinDialect capability gate).
func UpdateJoin(
	d dialect.Dialect, table string, sets []Assignment, joins []JoinSpec, _ JoinType, where Node, order []OrderTerm, limit, offset int,
) (query string, args []any, err error) {
	var b strings.Builder

	counter := &argCounter{}

	b.WriteString("UPDATE ")
	b.WriteString(d.QuoteIdent(table))

	writeSet(&b, d, sets, counter, &args)

	b.WriteString(" FROM ")
	writeFromTables(&b, d, joins)

	if err := writeJoinWhere(&b, d, table, joins, where, counter, &args); err != nil {
		return "", nil, err
	}

	if err := writeTail(&b, d, order, limit, offset, counter, &args); err != nil {
		return "", nil, err
	}

	return b.String(), args, nil
}

// DeleteJoin renders a DELETE against one or more joined tables:
// `DELETE FROM <table> USING <joined> WHERE ...` on Postgres. SQLite has no
// DELETE...USING, so a caller on SQLite must be rejected by the
// MutateJoinDialect capability gate before this renderer is reached. An
// ORDER BY / LIMIT / OFFSET tail renders after the WHERE.
func DeleteJoin(d dialect.Dialect, table string, joins []JoinSpec, _ JoinType, where Node, order []OrderTerm, limit, offset int) (query string, args []any, err error) {
	var b strings.Builder

	counter := &argCounter{}

	b.WriteString("DELETE FROM ")
	b.WriteString(d.QuoteIdent(table))
	b.WriteString(" USING ")
	writeFromTables(&b, d, joins)

	if err := writeJoinWhere(&b, d, table, joins, where, counter, &args); err != nil {
		return "", nil, err
	}

	if err := writeTail(&b, d, order, limit, offset, counter, &args); err != nil {
		return "", nil, err
	}

	return b.String(), args, nil
}

// UpdateJoinReturning renders UpdateJoin followed by an optional
// `RETURNING <cols>` clause. RETURNING binds no arguments, so the clause
// appearing after the FROM/USING ORDER/LIMIT tail leaves the Postgres $N
// numbering unchanged -- the same subtlety as writeReturning's "does not
// touch the counter" contract. returning columns are rendered only when
// non-empty.
func UpdateJoinReturning(
	d dialect.Dialect, table string, sets []Assignment, joins []JoinSpec, joinType JoinType, where Node, order []OrderTerm, limit, offset int, returning []string,
) (query string, args []any, err error) {
	q, a, err := UpdateJoin(d, table, sets, joins, joinType, where, order, limit, offset)
	if err != nil {
		return "", nil, err
	}

	if len(returning) == 0 {
		return q, a, nil
	}

	var b strings.Builder

	b.WriteString(q)
	writeReturning(&b, d, returning)

	return b.String(), a, nil
}

// DeleteJoinReturning renders DeleteJoin followed by an optional
// `RETURNING <cols>` clause; see UpdateJoinReturning.
func DeleteJoinReturning(
	d dialect.Dialect, table string, joins []JoinSpec, joinType JoinType, where Node, order []OrderTerm, limit, offset int, returning []string,
) (query string, args []any, err error) {
	q, a, err := DeleteJoin(d, table, joins, joinType, where, order, limit, offset)
	if err != nil {
		return "", nil, err
	}

	if len(returning) == 0 {
		return q, a, nil
	}

	var b strings.Builder

	b.WriteString(q)
	writeReturning(&b, d, returning)

	return b.String(), a, nil
}
