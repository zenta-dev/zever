// Package render (this file) extends the package with Postgres array
// quantifier predicate rendering for the Column.EqAny/NeqAny/EqAll/
// NeqAll helpers. A KindArray node's base column is compared against an
// ARRAY[...] constructor with one bound placeholder per element, using the
// `= ANY`, `<> ANY`, `= ALL` or `<> ALL` quantifier selected by the node's
// Op. The syntax is Postgres-only, so the dialect must implement
// dialect.ArrayDialect and report support; every other dialect gets the
// typed dialect.ErrUnsupportedByDialect rather than invalid SQL. See
// docs-db-orm/DESIGN.md §5's capability row and render.go's KindArray.
package render

import (
	"fmt"
	"strings"

	"github.com/zenta-dev/zever/orm/dialect"
)

// renderArray renders one KindArray predicate: `col <op> <quant>(ARRAY[ph,
// ...])`. One placeholder is emitted per element, in value order, so
// Postgres-style $N numbering stays sequential across the enclosing
// statement. An empty element list renders a constant boolean rather than
// the invalid `ARRAY[]` literal: `x = ANY(empty)` and `x <> ANY(empty)` are
// always false, while `x = ALL(empty)` and `x <> ALL(empty)` are always
// true. err is non-nil for a dialect without array support, an operator that
// is not an array quantifier, or a non-[]any payload -- all typed
// rendering-time failures, never silently-wrong SQL.
func renderArray(d dialect.Dialect, n Node, counter *argCounter) (clause string, args []any, err error) {
	if !supportsArrayPredicates(d) {
		return "", nil, fmt.Errorf("orm/render: %w: dialect %q does not support array predicates (EQ ANY / EQ ALL / NEQ ALL)", dialect.ErrUnsupportedByDialect, d.Name())
	}

	symbol, quantifier, err := arrayOp(n.Op)
	if err != nil {
		return "", nil, err
	}

	vs, ok := n.Value.([]any)
	if !ok {
		return "", nil, fmt.Errorf("orm/render: array predicate value of type %T is not a []any", n.Value)
	}

	if len(vs) == 0 {
		if quantifier == "ANY" {
			return "1 = 0", nil, nil
		}

		return "1 = 1", nil, nil
	}

	phs := make([]string, len(vs))
	for i := range vs {
		phs[i] = d.Placeholder(counter.next())
	}

	col := quoteColumn(d, n.Column)

	return col + " " + symbol + " " + quantifier + "(ARRAY[" + strings.Join(phs, ", ") + "])", vs, nil
}

// arrayOp maps a KindArray node's Op to its comparison symbol and ANY/ALL
// quantifier. The only valid ops are the four built by Column.EqAny/NeqAny/
// EqAll/NeqAll; anything else is a caller bug surfaced as a typed error.
func arrayOp(op Op) (symbol, quantifier string, err error) {
	switch op { //nolint:exhaustive // only the four array quantifier ops are valid; the rest error in default
	case OpEqAny:
		return "=", "ANY", nil
	case OpNeqAny:
		return "<>", "ANY", nil
	case OpEqAll:
		return "=", "ALL", nil
	case OpNeqAll:
		return "<>", "ALL", nil
	default:
		return "", "", fmt.Errorf("orm/render: operator %d is not an array quantifier (use Column.EqAny/NeqAny/EqAll/NeqAll)", op)
	}
}

// supportsArrayPredicates reports whether d implements
// dialect.ArrayDialect and reports support. A dialect that does not
// implement the interface at all (e.g. a base-only dialect) is treated as
// unsupported, matching the repo's capability-gate convention (see
// supportsNullsOrdering).
func supportsArrayPredicates(d dialect.Dialect) bool {
	ad, ok := d.(dialect.ArrayDialect)

	return ok && ad.SupportsArrayPredicates()
}
