package render

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// MergeAssignment is one target-column assignment in a MERGE clause. Exactly
// one of Source/Value is meaningful: a non-empty Source names a column on the
// MERGE source (rendered qualified as `source.col`, binding nothing), while
// an empty Source means Value is a literal bound as a placeholder. It is
// render's erased view of the builder's MergeAssignment, the same conversion pattern as
// render.Assignment and render.JoinSpec.
type MergeAssignment struct {
	Column string
	Source string
	Value  any
}

// MergeAction is the action a MERGE WHEN clause takes.
type MergeAction int

const (
	// MergeUpdate is `WHEN MATCHED THEN UPDATE SET ...` (matched only).
	MergeUpdate MergeAction = iota
	// MergeDelete is `WHEN MATCHED THEN DELETE` (matched only).
	MergeDelete
	// MergeInsert is `WHEN NOT MATCHED THEN INSERT ...` (not-matched only).
	MergeInsert
)

// MergeWhen is one WHEN clause of a MERGE: Matched selects the
// WHEN MATCHED / WHEN NOT MATCHED arm, Action selects the verb, and Sets is
// the target assignment list (UPDATE and INSERT only; ignored for DELETE).
type MergeWhen struct {
	Matched bool
	Action  MergeAction
	Sets    []MergeAssignment
}

// Merge renders a SQL-standard MERGE statement:
//
//	MERGE INTO <target> USING <source> ON <on...>
//	WHEN MATCHED THEN UPDATE SET ... | DELETE
//	WHEN NOT MATCHED THEN INSERT (...) VALUES (...) | DEFAULT VALUES
//
// The ON conditions are the same equi-join specs the mutation join renderers
// use, rendered `target.col = source.col` and ANDed. Only literal
// assignment values bind arguments (source-column references render as
// qualified identifiers); placeholders are numbered with the shared
// argCounter so Postgres `$N` numbering is continuous across every clause.
//
// The renderer fails closed on a MERGE it cannot render validly -- no target/
// source, no ON condition, no WHEN clause, an UPDATE/DELETE in a
// WHEN NOT MATCHED arm (or INSERT in WHEN MATCHED), an UPDATE with no
// assignments, or a DELETE carrying assignments -- rather than emitting SQL
// the server would reject. The caller gates the dialect (MERGE is Postgres 15+
// only; see dialect.MergeDialect).
func Merge(d dialect.Dialect, target, source string, on []JoinSpec, whens []MergeWhen) (query string, args []any, err error) {
	if target == "" || source == "" {
		return "", nil, errors.New("orm/render: Merge: target and source table names are required")
	}

	if len(on) == 0 {
		return "", nil, errors.New("orm/render: Merge: at least one ON condition is required")
	}

	if len(whens) == 0 {
		return "", nil, errors.New("orm/render: Merge: at least one WHEN clause is required")
	}

	var b strings.Builder

	counter := &argCounter{}

	b.WriteString("MERGE INTO ")
	b.WriteString(d.QuoteIdent(target))
	b.WriteString(" USING ")
	b.WriteString(d.QuoteIdent(source))
	b.WriteString(" ON ")
	b.WriteString(strings.Join(joinConditions(d, target, on), " AND "))

	for _, w := range whens {
		if err := writeMergeWhen(&b, d, source, w, counter, &args); err != nil {
			return "", nil, err
		}
	}

	return b.String(), args, nil
}

// writeMergeWhen renders one MERGE WHEN clause, validating the arm/action
// pairing so an invalid clause is a rendering error rather than invalid SQL.
func writeMergeWhen(b *strings.Builder, d dialect.Dialect, source string, w MergeWhen, counter *argCounter, args *[]any) error {
	switch w.Action {
	case MergeUpdate:
		if !w.Matched {
			return errors.New("orm/render: Merge: UPDATE is only valid in WHEN MATCHED")
		}

		if len(w.Sets) == 0 {
			return errors.New("orm/render: Merge: WHEN MATCHED THEN UPDATE requires at least one assignment")
		}

		b.WriteString(" WHEN MATCHED THEN UPDATE SET ")
		writeMergeSets(b, d, source, w.Sets, counter, args)
	case MergeDelete:
		if !w.Matched {
			return errors.New("orm/render: Merge: DELETE is only valid in WHEN MATCHED")
		}

		if len(w.Sets) > 0 {
			return errors.New("orm/render: Merge: WHEN MATCHED THEN DELETE takes no assignments")
		}

		b.WriteString(" WHEN MATCHED THEN DELETE")
	case MergeInsert:
		if w.Matched {
			return errors.New("orm/render: Merge: INSERT is only valid in WHEN NOT MATCHED")
		}

		b.WriteString(" WHEN NOT MATCHED THEN INSERT ")

		if len(w.Sets) == 0 {
			b.WriteString("DEFAULT VALUES")

			return nil
		}

		cols := make([]string, len(w.Sets))
		vals := make([]string, len(w.Sets))

		for i, s := range w.Sets {
			cols[i] = d.QuoteIdent(s.Column)
			vals[i] = writeMergeValue(d, source, s, counter, args)
		}

		b.WriteString("(")
		b.WriteString(strings.Join(cols, ", "))
		b.WriteString(") VALUES (")
		b.WriteString(strings.Join(vals, ", "))
		b.WriteString(")")
	default:
		return fmt.Errorf("orm/render: Merge: unknown action %d", w.Action)
	}

	return nil
}

// writeMergeSets renders `col = value, ...` for a MERGE UPDATE SET clause.
func writeMergeSets(b *strings.Builder, d dialect.Dialect, source string, sets []MergeAssignment, counter *argCounter, args *[]any) {
	parts := make([]string, len(sets))
	for i, s := range sets {
		parts[i] = d.QuoteIdent(s.Column) + " = " + writeMergeValue(d, source, s, counter, args)
	}

	b.WriteString(strings.Join(parts, ", "))
}

// writeMergeValue renders one MERGE assignment's right-hand side: the source
// column qualified by source (binding nothing) when Source is set, else a
// placeholder with its literal value appended to args.
func writeMergeValue(d dialect.Dialect, source string, s MergeAssignment, counter *argCounter, args *[]any) string {
	if s.Source != "" {
		return quoteColumn(d, source+"."+s.Source)
	}

	*args = append(*args, s.Value)

	return d.Placeholder(counter.next())
}
