// Package postgres implements dialect.Dialect for Postgres.
package postgres

import "strconv"

// Dialect renders numbered placeholders ($1, $2, ...) and double-quoted
// identifiers, matching Postgres's syntax. It also records the server
// version it was constructed for (see version.go) so version-gated
// capability checks (e.g. MERGE on 15+, CTE MATERIALIZED on 12+, recursive
// SEARCH/CYCLE on 14+) can branch on it.
type Dialect struct {
	version version
}

// New returns a Postgres Dialect defaulting to 16.0.0, a modern release far
// above every version floor the capability matrix gates on (CTE
// MATERIALIZED on 12.0+, recursive SEARCH/CYCLE on 14.0+, MERGE on 15.0+).
// NewWithVersion (see version.go) exists for callers pinned to an older
// server.
func New() Dialect {
	return Dialect{version: version{major: 16}}
}

// Name returns the dialect's name.
func (Dialect) Name() string { return "postgres" }

// Placeholder returns the nth (1-indexed) positional placeholder, e.g. $1.
func (Dialect) Placeholder(n int) string {
	return "$" + strconv.Itoa(n)
}

// QuoteIdent double-quotes an identifier for Postgres. It does not escape
// embedded quote characters; callers never pass caller-controlled strings
// through this path, so this is a known, accepted limitation rather than
// an oversight.
func (Dialect) QuoteIdent(s string) string {
	return `"` + s + `"`
}

// SupportsCTE reports that Postgres supports common table expressions.
func (Dialect) SupportsCTE() bool { return true }

// SupportsRecursive reports that Postgres supports WITH RECURSIVE.
func (Dialect) SupportsRecursive() bool { return true }

// SupportsIntersectExcept reports that Postgres supports INTERSECT/EXCEPT.
func (Dialect) SupportsIntersectExcept() bool { return true }

// SupportsReturning reports that Postgres supports a RETURNING clause.
func (Dialect) SupportsReturning() bool { return true }

// SupportsConflictTargetWhere reports that Postgres supports a partial-index
// predicate on the ON CONFLICT conflict target (`ON CONFLICT (cols) WHERE
// <predicate>`), which names the partial unique index to arbitrate on.
func (Dialect) SupportsConflictTargetWhere() bool { return true }

// SupportsConflictUpdateWhere reports that Postgres supports a condition on
// the DO UPDATE SET clause (`DO UPDATE SET ... WHERE <predicate>`).
func (Dialect) SupportsConflictUpdateWhere() bool { return true }

// SupportsExplainAnalyze reports that Postgres supports EXPLAIN ANALYZE.
func (Dialect) SupportsExplainAnalyze() bool { return true }

// SupportsMerge reports whether this Postgres server version supports the
// SQL-standard MERGE statement, which arrived in Postgres 15. New defaults
// to 16.x, so the common path reports true; a pre-15 server (constructed
// via NewWithVersion) reports false.
func (d Dialect) SupportsMerge() bool {
	return d.version.atLeast(15, 0, 0)
}

// SupportsNullsOrdering reports that Postgres supports the
// `NULLS FIRST` / `NULLS LAST` ORDER BY modifier. It is native Postgres
// syntax and available at every supported version.
func (Dialect) SupportsNullsOrdering() bool { return true }

// SupportsArrayPredicates reports that Postgres supports the typed array
// quantifier predicates `= ANY(...)`/`<> ANY(...)`/`= ALL(...)`/
// `<> ALL(...)` and the ARRAY[...] constructor they render. Both are native
// Postgres syntax, so this is unconditional.
func (Dialect) SupportsArrayPredicates() bool { return true }

// SupportsRightJoin reports that Postgres supports RIGHT JOIN.
func (Dialect) SupportsRightJoin() bool { return true }

// SupportsFullJoin reports that Postgres supports FULL JOIN.
func (Dialect) SupportsFullJoin() bool { return true }

// SupportsUpdateJoin reports that Postgres supports `UPDATE ... FROM`.
func (Dialect) SupportsUpdateJoin() bool { return true }

// SupportsDeleteJoin reports that Postgres supports `DELETE ... USING`.
func (Dialect) SupportsDeleteJoin() bool { return true }

// SupportsLeftMutateJoin reports that Postgres supports a LEFT JOIN inside
// an UPDATE/DELETE. The FROM/USING forms can only express an inner join,
// so this always reports false.
func (Dialect) SupportsLeftMutateJoin() bool { return false }

// SupportsForUpdate reports that Postgres supports `SELECT ... FOR UPDATE`.
func (Dialect) SupportsForUpdate() bool { return true }

// SupportsForShare reports that Postgres supports `SELECT ... FOR SHARE`.
func (Dialect) SupportsForShare() bool { return true }

// SupportsNoWait reports that Postgres supports the `NOWAIT` locking suffix
// on FOR UPDATE / FOR SHARE.
func (Dialect) SupportsNoWait() bool { return true }

// SupportsSkipLocked reports that Postgres supports the `SKIP LOCKED`
// locking suffix on FOR UPDATE / FOR SHARE.
func (Dialect) SupportsSkipLocked() bool { return true }

// SupportsUpdateOrderLimit reports that Postgres supports an
// `ORDER BY ... LIMIT` tail on a single-table UPDATE. The Postgres grammar
// has no ORDER BY/LIMIT/OFFSET for UPDATE or DELETE, so this, and the rest
// of the MutateOrderDialect method set, always reports false.
func (Dialect) SupportsUpdateOrderLimit() bool { return false }

// SupportsDeleteOrderLimit reports that Postgres supports an
// `ORDER BY ... LIMIT` tail on a DELETE. See SupportsUpdateOrderLimit.
func (Dialect) SupportsDeleteOrderLimit() bool { return false }

// SupportsMutateOffset reports that Postgres supports OFFSET on an
// UPDATE/DELETE. See SupportsUpdateOrderLimit.
func (Dialect) SupportsMutateOffset() bool { return false }

// SupportsJoinedMutateOrderLimit reports that Postgres supports an ORDER BY
// / LIMIT tail on a joined UPDATE/DELETE. See SupportsUpdateOrderLimit.
func (Dialect) SupportsJoinedMutateOrderLimit() bool { return false }
