package render

import (
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// SelectSource is render's erased view of the SELECT feeding an
// `INSERT ... SELECT`: the same fields render.Select consumes, plus the
// source table name. The builder converts a typed query into this shape at
// .Select() build time (see the insert-select builder), keeping render decoupled
// from the orm root package -- the same erasure pattern as render.Assignment and
// render.JoinSpec.
type SelectSource struct {
	Table   string
	Columns []string
	Where   Node
	Order   []OrderTerm
	Limit   int
	Offset  int
}

// beginInsertSelect renders the shared `INSERT INTO <table> (<columns>) ` head
// followed by the source SELECT's text, and returns a builder positioned for
// the optional conflict/returning tail along with the statement's arguments
// so far. It reuses render.Select (so the SELECT body, its joins-free
// dialect rendering and its shape cache are shared with a standalone
// query), and reuses render's argCounter so Postgres `$N` numbering is
// continuous across the whole statement.
//
// An INSERT binds no arguments of its own, so the SELECT's own placeholders
// are already `$1..$k`; the returned counter is therefore seeded to the
// SELECT's argument count so a following ON CONFLICT DO UPDATE SET /
// ON DUPLICATE KEY UPDATE assignment continues at `$k+1`. The args slice is
// the SELECT's bound arguments in placeholder order.
func beginInsertSelect(
	d dialect.Dialect, table string, columns []string, src SelectSource,
) (b *strings.Builder, counter *argCounter, args []any, err error) {
	selectSQL, selectArgs, err := Select(d, src.Table, src.Columns, src.Where, src.Order, src.Limit, src.Offset)
	if err != nil {
		return nil, nil, nil, err
	}

	quoted := make([]string, len(columns))
	for i, c := range columns {
		quoted[i] = d.QuoteIdent(c)
	}

	var out strings.Builder

	out.WriteString("INSERT INTO ")
	out.WriteString(d.QuoteIdent(table))
	out.WriteString(" (")
	out.WriteString(strings.Join(quoted, ", "))
	out.WriteString(") ")
	out.WriteString(selectSQL)

	return &out, &argCounter{n: len(selectArgs)}, selectArgs, nil
}

// InsertSelect renders a plain `INSERT INTO <table> (<columns>) SELECT ...`
// statement with an optional `RETURNING <cols>` clause. There is no VALUES
// keyword for this form; the source SELECT is rendered by render.Select
// against the same dialect. returning columns are rendered only when
// non-empty; args are the SELECT's bound arguments.
func InsertSelect(
	d dialect.Dialect, table string, columns []string, src SelectSource, returning []string,
) (query string, args []any, err error) {
	b, _, args, err := beginInsertSelect(d, table, columns, src)
	if err != nil {
		return "", nil, err
	}

	writeReturning(b, d, returning)

	return b.String(), args, nil
}

// InsertSelectOnConflict renders `INSERT INTO <table> (<columns>) SELECT ...
// ON CONFLICT [(target)] DO UPDATE SET ... | DO NOTHING [RETURNING ...]` for
// a dialect that supports ON CONFLICT (Postgres, SQLite). A nil sets renders
// DO NOTHING; a non-nil sets renders DO UPDATE SET, with every assignment's
// value bound AFTER the SELECT's arguments -- so the Postgres placeholder
// numbering runs `$1..$k` for the SELECT and then `$k+1..` for the
// assignments. returning is rendered only if non-empty.
//
// It is the no-predicate projection of InsertSelectOnConflictWhere and
// delegates to it, so the two can never render differently.
func InsertSelectOnConflict(
	d dialect.Dialect, table string, columns []string, src SelectSource, target []string, sets []Assignment, returning []string,
) (query string, args []any, err error) {
	return InsertSelectOnConflictWhere(d, table, columns, src, target, ConflictWhere{}, sets, returning)
}

// InsertSelectOnConflictWhere is InsertSelectOnConflict with the two
// optional upsert predicates (see InsertOnConflictWhere). Both render AFTER
// the source SELECT body, so the Postgres `$N` numbering runs through the
// SELECT's own arguments first, then where.Target's, then the SET
// assignments, then where.Update's.
func InsertSelectOnConflictWhere(
	d dialect.Dialect, table string, columns []string, src SelectSource, target []string, where ConflictWhere, sets []Assignment, returning []string,
) (query string, args []any, err error) {
	b, counter, args, err := beginInsertSelect(d, table, columns, src)
	if err != nil {
		return "", nil, err
	}

	if err := writeOnConflict(b, d, target, where, sets, counter, &args); err != nil {
		return "", nil, err
	}

	writeReturning(b, d, returning)

	return b.String(), args, nil
}
