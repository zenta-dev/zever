// Package sqlite implements dialect.Dialect for SQLite.
package sqlite

// Dialect renders a single "?" placeholder for every argument and
// double-quoted identifiers, matching SQLite's syntax. It also records the
// SQLite library version it was constructed for (see version.go) so
// version-gated capability checks can branch on it.
type Dialect struct {
	version version
}

// New returns a SQLite Dialect defaulting to 3.46.0, far above every
// capability floor the matrix gates on, including the 3.30.0 NULLS
// FIRST/LAST and 3.39.0 RIGHT/FULL JOIN floors.
func New() Dialect {
	return Dialect{version: version{major: 3, minor: 46, patch: 0}}
}

// NewWithVersion returns a SQLite Dialect for a specific library version,
// e.g. "3.46.0" or "3.30.0". An unparsable version string is an error,
// never a silent fallback or panic: capability decisions must not be built
// on a guessed version. NewWithVersion exists for the version-gate tests
// and for drivers whose bundled SQLite is older than the default.
func NewWithVersion(s string) (Dialect, error) {
	v, err := parseVersion(s)
	if err != nil {
		return Dialect{}, err
	}

	return Dialect{version: v}, nil
}

// Name returns the dialect's name.
func (Dialect) Name() string { return "sqlite" }

// Placeholder returns the placeholder for the nth argument. SQLite uses the
// same "?" token regardless of position.
func (Dialect) Placeholder(int) string { return "?" }

// QuoteIdent double-quotes an identifier for SQLite.
func (Dialect) QuoteIdent(s string) string { return `"` + s + `"` }

// SupportsCTE reports that SQLite supports common table expressions.
func (Dialect) SupportsCTE() bool { return true }

// SupportsRecursive reports that SQLite supports WITH RECURSIVE.
func (Dialect) SupportsRecursive() bool { return true }

// SupportsIntersectExcept reports that SQLite supports INTERSECT/EXCEPT.
func (Dialect) SupportsIntersectExcept() bool { return true }

// SupportsReturning reports that SQLite supports a RETURNING clause.
func (Dialect) SupportsReturning() bool { return true }

// SupportsConflictTargetWhere reports that SQLite supports a partial-index
// predicate on the ON CONFLICT conflict target (`ON CONFLICT (cols) WHERE
// <predicate>`), which names the partial unique index to arbitrate on.
func (Dialect) SupportsConflictTargetWhere() bool { return true }

// SupportsConflictUpdateWhere reports that SQLite supports a condition on
// the DO UPDATE SET clause (`DO UPDATE SET ... WHERE <predicate>`).
func (Dialect) SupportsConflictUpdateWhere() bool { return true }

// SupportsExplainAnalyze reports that SQLite supports EXPLAIN ANALYZE. No
// SQLite version has EXPLAIN ANALYZE (only plain EXPLAIN and EXPLAIN QUERY
// PLAN), so this always reports false.
func (Dialect) SupportsExplainAnalyze() bool { return false }

// SupportsMerge reports whether this SQLite library version supports the
// SQL-standard MERGE statement. No SQLite version has MERGE, so this always
// reports false.
func (Dialect) SupportsMerge() bool { return false }

// SupportsNullsOrdering reports whether this SQLite library version supports
// the `NULLS FIRST` / `NULLS LAST` ORDER BY modifier, which arrived in
// SQLite 3.30.0. Older versions reject the syntax. New defaults to 3.46.0,
// so the common path reports true.
func (d Dialect) SupportsNullsOrdering() bool {
	return d.version.atLeast(3, 30, 0)
}

// SupportsArrayPredicates reports whether SQLite supports the typed array
// quantifier predicates (`= ANY(...)`, `<> ALL(...)`, ...). SQLite has no
// ARRAY constructor and no ANY/ALL array quantifiers at any version, so
// this always reports false.
func (Dialect) SupportsArrayPredicates() bool { return false }

// SupportsRightJoin reports whether this SQLite library version supports
// RIGHT [OUTER] JOIN, which became core SQL in SQLite 3.39.0. Older
// versions reject the syntax. New defaults to 3.46.0, so the common path
// reports true.
func (d Dialect) SupportsRightJoin() bool {
	return d.version.atLeast(3, 39, 0)
}

// SupportsFullJoin reports whether this SQLite library version supports
// FULL [OUTER] JOIN, which arrived with RIGHT JOIN in SQLite 3.39.0. See
// SupportsRightJoin for the version gate and its rationale.
func (d Dialect) SupportsFullJoin() bool {
	return d.version.atLeast(3, 39, 0)
}

// SupportsUpdateJoin reports that SQLite supports `UPDATE ... FROM`.
func (Dialect) SupportsUpdateJoin() bool { return true }

// SupportsDeleteJoin reports that SQLite supports a joined DELETE. No
// SQLite version has a `DELETE ... USING` clause, so this always reports
// false.
func (Dialect) SupportsDeleteJoin() bool { return false }

// SupportsLeftMutateJoin reports that SQLite supports a LEFT JOIN inside
// an UPDATE/DELETE. SQLite's `UPDATE ... FROM` can only express an inner
// join (there is no DELETE-join at all), so this always reports false.
func (Dialect) SupportsLeftMutateJoin() bool { return false }

// SupportsForUpdate reports that SQLite supports `SELECT ... FOR UPDATE`.
// SQLite has no row-level locking, so this, and the rest of the
// LockingDialect method set, always reports false.
func (Dialect) SupportsForUpdate() bool { return false }

// SupportsForShare reports that SQLite supports `SELECT ... FOR SHARE`. See
// SupportsForUpdate for why this always reports false.
func (Dialect) SupportsForShare() bool { return false }

// SupportsNoWait reports that SQLite supports the `NOWAIT` locking suffix.
// See SupportsForUpdate for why this always reports false.
func (Dialect) SupportsNoWait() bool { return false }

// SupportsSkipLocked reports that SQLite supports the `SKIP LOCKED`
// locking suffix. See SupportsForUpdate for why this always reports false.
func (Dialect) SupportsSkipLocked() bool { return false }

// SupportsUpdateOrderLimit reports that SQLite supports an
// `ORDER BY ... LIMIT` tail on a single-table UPDATE. SQLite only enables
// UPDATE/DELETE ORDER BY/LIMIT/OFFSET when compiled with
// SQLITE_ENABLE_UPDATE_DELETE_LIMIT, which the bundled driver does not set,
// so this, and the rest of the MutateOrderDialect method set, always
// reports false.
func (Dialect) SupportsUpdateOrderLimit() bool { return false }

// SupportsDeleteOrderLimit reports that SQLite supports an
// `ORDER BY ... LIMIT` tail on a DELETE. See SupportsUpdateOrderLimit.
func (Dialect) SupportsDeleteOrderLimit() bool { return false }

// SupportsMutateOffset reports that SQLite supports OFFSET on an
// UPDATE/DELETE. See SupportsUpdateOrderLimit.
func (Dialect) SupportsMutateOffset() bool { return false }

// SupportsJoinedMutateOrderLimit reports that SQLite supports an ORDER BY /
// LIMIT tail on a joined UPDATE/DELETE. See SupportsUpdateOrderLimit.
func (Dialect) SupportsJoinedMutateOrderLimit() bool { return false }
