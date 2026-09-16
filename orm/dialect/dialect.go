package dialect

import "errors"

// Dialect abstracts the SQL syntax differences renderers need to know
// about: how placeholders are written and how identifiers are quoted.
type Dialect interface {
	Name() string
	Placeholder(n int) string
	QuoteIdent(s string) string
}

// CTEDialect is implemented by dialects that support common table
// expressions (WITH ... AS (...)). SupportsCTE gates a plain WITH;
// WITH RECURSIVE additionally requires SupportsRecursive to report true.
type CTEDialect interface {
	Dialect
	SupportsCTE() bool
	SupportsRecursive() bool
}

// SetOpDialect is implemented by dialects that support the INTERSECT and
// EXCEPT set operators. UNION and UNION ALL are universal SQL and are
// deliberately NOT gated by this interface.
type SetOpDialect interface {
	Dialect
	SupportsIntersectExcept() bool
}

// ReturningDialect is implemented by dialects that support a RETURNING
// clause on INSERT/UPDATE/DELETE statements.
type ReturningDialect interface {
	Dialect
	SupportsReturning() bool
}

// ExplainDialect is implemented by dialects that support EXPLAIN ANALYZE.
// Plain EXPLAIN is universal SQL and is deliberately NOT gated by this
// interface.
type ExplainDialect interface {
	Dialect
	SupportsExplainAnalyze() bool
}

// MergeDialect is implemented by dialects that support the SQL-standard
// MERGE statement (MERGE INTO target USING source ON ... WHEN MATCHED THEN
// ... WHEN NOT MATCHED THEN ...). SupportsMerge is version-gated where the
// engine added MERGE late. A requested MERGE on an unsupported dialect
// returns a typed ErrUnsupportedByDialect at execution time, never invalid
// SQL.
type MergeDialect interface {
	Dialect
	SupportsMerge() bool
}

// JSONDialect is implemented by dialects whose JSON text-extraction
// rendering is version-gated, exposing whether the `->>` shorthand is
// available. Neither in-tree dialect implements it; renderers treat a
// non-JSONDialect as always supporting `->>`.
type JSONDialect interface {
	Dialect
	SupportsJSONArrowText() bool
}

// JSONTableDialect is implemented by dialects that support a table-valued
// JSON source function usable in a FROM clause. SupportsJSONTable is an
// explicit method, not bare interface presence, so a dialect can implement
// the set and report false below its version floor. An unsupported dialect
// is rejected with a typed ErrUnsupportedByDialect, never invalid SQL.
type JSONTableDialect interface {
	Dialect
	SupportsJSONTable() bool
}

// JSONEachDialect is implemented by dialects that support SQLite's json1
// table-valued functions `json_each(doc[, path])` and
// `json_tree(doc[, path])` as a FROM/JOIN source. SupportsJSONEach is an
// explicit method, not bare interface presence, so every in-tree dialect
// implements the method set and reports its own answer. An unsupported
// dialect is rejected with a typed ErrUnsupportedByDialect, never invalid
// SQL.
type JSONEachDialect interface {
	Dialect
	SupportsJSONEach() bool
}

// JSONSetReturningDialect is implemented by dialects that support a
// set-returning JSON function as a FROM/JOIN source, such as Postgres's
// `jsonb_array_elements(doc)` / `jsonb_array_elements_text(doc)`, which
// correlate implicitly to earlier FROM entries. SupportsJSONSetReturning is
// an explicit method rather than bare interface presence, so every in-tree
// dialect reports its own answer. An unsupported dialect is rejected with a
// typed ErrUnsupportedByDialect.
type JSONSetReturningDialect interface {
	Dialect
	SupportsJSONSetReturning() bool
}

// JoinCapabilities is implemented by dialects that support the outer join
// keywords beyond LEFT. RIGHT JOIN keeps every right-side row; FULL JOIN
// keeps both unmatched sides. SupportsRightJoin and SupportsFullJoin are
// separate explicit methods, not just interface presence, because every
// supported dialect implements this method set, so a per-keyword boolean is
// what actually rejects the unsupported one with a typed
// ErrUnsupportedByDialect.
type JoinCapabilities interface {
	Dialect
	SupportsRightJoin() bool
	SupportsFullJoin() bool
}

// MutateJoinDialect is implemented by dialects that support joining another
// table in an UPDATE or DELETE statement. SupportsUpdateJoin gates
// `UPDATE ... FROM`; SupportsDeleteJoin gates `DELETE ... USING`.
// SupportsLeftMutateJoin additionally gates a LEFT JOIN inside those
// statements, which the FROM/USING form cannot express. Each unsupported
// combination is rejected with a typed ErrUnsupportedByDialect at execution
// time.
type MutateJoinDialect interface {
	Dialect
	SupportsUpdateJoin() bool
	SupportsDeleteJoin() bool
	SupportsLeftMutateJoin() bool
}

// MutateOrderDialect is implemented by dialects that support an
// `ORDER BY ... [LIMIT ... [OFFSET ...]]` tail on an UPDATE or DELETE
// statement. The four booleans mirror MutateJoinDialect's explicit-method
// shape rather than interface presence, so each statement kind is gated
// separately. No in-tree dialect supports any form; an unsupported
// combination returns a typed ErrUnsupportedByDialect at execution time,
// never invalid SQL handed to the engine.
type MutateOrderDialect interface {
	Dialect
	SupportsUpdateOrderLimit() bool
	SupportsDeleteOrderLimit() bool
	SupportsMutateOffset() bool
	SupportsJoinedMutateOrderLimit() bool
}

// ConflictWhereDialect is implemented by dialects that support the two
// optional WHERE predicates of an upsert. The booleans are explicit
// per-clause methods, not bare interface presence, because a dialect can
// support one form without the other. SupportsConflictTargetWhere gates the
// partial-index predicate on the conflict target; SupportsConflictUpdateWhere
// gates a condition on the DO UPDATE SET clause. A requested but
// unsupported predicate is rejected with a typed ErrUnsupportedByDialect
// rather than silently dropped.
type ConflictWhereDialect interface {
	Dialect
	SupportsConflictTargetWhere() bool
	SupportsConflictUpdateWhere() bool
}

// LockingDialect is implemented by dialects that support row-level locking
// clauses on a SELECT (FOR UPDATE / FOR SHARE, optionally followed by
// NOWAIT or SKIP LOCKED). The four booleans are explicit per-feature
// methods rather than bare interface presence, because every supported
// dialect implements this method set, so a per-clause boolean is what
// actually rejects an unsupported request with a typed
// ErrUnsupportedByDialect.
type LockingDialect interface {
	Dialect
	SupportsForUpdate() bool
	SupportsForShare() bool
	SupportsNoWait() bool
	SupportsSkipLocked() bool
}

// NullsOrderDialect is implemented by dialects that support the SQL
// `NULLS FIRST` / `NULLS LAST` ORDER BY modifier. It is one explicit method
// rather than bare interface presence, so a dialect can report support
// conditionally on its version. Callers needing the ordering on a dialect
// without it build the portable equivalent with a CASE expression.
type NullsOrderDialect interface {
	Dialect
	SupportsNullsOrdering() bool
}

// DistinctOnDialect is implemented by dialects that support the Postgres
// `SELECT DISTINCT ON (cols)` extension. A DISTINCT ON request on a dialect
// without the syntax is rejected with a typed ErrUnsupportedByDialect
// rather than emulated behind the caller's back.
type DistinctOnDialect interface {
	Dialect
	SupportsDistinctOn() bool
}

// ExtendedLockingDialect is implemented by dialects that support the
// Postgres-only row-lock extensions: the weaker `FOR NO KEY UPDATE` /
// `FOR KEY SHARE` strengths and the `FOR ... OF <table>` target list. It is
// deliberately a separate interface from LockingDialect so the base
// `FOR UPDATE`/`FOR SHARE` capability matrix stays unchanged and the extras
// gate independently.
type ExtendedLockingDialect interface {
	Dialect
	SupportsForNoKeyUpdate() bool
	SupportsForKeyShare() bool
	SupportsForUpdateOf() bool
}

// TablesampleDialect is implemented by dialects that support the
// `TABLESAMPLE <method> (<percentage>)` FROM-clause extension. A
// TABLESAMPLE request on a dialect without it is a typed
// ErrUnsupportedByDialect.
type TablesampleDialect interface {
	Dialect
	SupportsTablesample() bool
}

// LateralJoinDialect is implemented by dialects that support a LATERAL
// derived-table join, letting a FROM-clause subquery reference the columns
// of a table to its left. SupportsLateral is one explicit method, not bare
// interface presence, because one in-tree dialect implements the rest of
// the capability set yet has no LATERAL syntax at any version. A dialect
// lacking the capability returns the typed ErrUnsupportedByDialect, never a
// panic or a silently-degraded join.
type LateralJoinDialect interface {
	Dialect
	SupportsLateral() bool
}

// AggregateGroupingDialect is implemented by dialects that support the
// aggregate/grouping extensions of grouped SELECT statements: a FILTER
// (WHERE ...) clause on an aggregate, and the ROLLUP/CUBE/GROUPING SETS
// GROUP BY constructs. The booleans are explicit per-feature methods, not
// bare interface presence, because a dialect can support one without the
// others, and every one is rejected with a typed ErrUnsupportedByDialect,
// never a silent wrong-SQL fallback.
type AggregateGroupingDialect interface {
	Dialect
	SupportsAggregateFilter() bool
	SupportsRollup() bool
	SupportsCube() bool
	SupportsGroupingSets() bool
}

// OrderedAggregateDialect is implemented by dialects that support
// argument-last ordered aggregates: a concatenating/collecting aggregate
// whose ORDER BY sits inside the function call (`array_agg(x ORDER BY y)`,
// `string_agg(x, d ORDER BY y)`, `group_concat(x ORDER BY y)`). The four
// booleans are explicit per-feature methods, not bare interface presence,
// because the function families are disjoint and the syntax is not
// portable. Each unsupported combination is a typed ErrUnsupportedByDialect,
// never silently-wrong SQL.
type OrderedAggregateDialect interface {
	Dialect
	SupportsArrayAgg() bool
	SupportsStringAgg() bool
	SupportsGroupConcat() bool
	SupportsOrderedAggregates() bool
}

// ArrayDialect is implemented by dialects that support typed array
// quantifier predicates: `col = ANY(array)`, `col <> ANY(array)`,
// `col = ALL(array)` and `col <> ALL(array)`. SupportsArrayPredicates is an
// explicit method, not bare interface presence, so every in-tree dialect
// implements this method set and reports its own answer, keeping the
// capability decision visible in the dialect rather than implied by which
// interfaces happen to be satisfied.
type ArrayDialect interface {
	Dialect
	SupportsArrayPredicates() bool
}

// WindowFrameDialect is implemented by dialects that support a window
// frame clause on an OVER expression (`ROWS`/`RANGE`/`GROUPS BETWEEN
// <start> AND <end>`). The three booleans are explicit per-mode methods,
// not bare interface presence, because a dialect can support some modes
// and not others, and a requested but unsupported mode must be rejected
// with a typed ErrUnsupportedByDialect rather than silently dropped or
// rendered as SQL the engine rejects.
type WindowFrameDialect interface {
	Dialect
	SupportsWindowFrameRows() bool
	SupportsWindowFrameRange() bool
	SupportsWindowFrameGroups() bool
}

// CTEMaterializationDialect is implemented by dialects that support the
// `MATERIALIZED` / `NOT MATERIALIZED` modifier on a common table expression
// definition (`WITH name AS MATERIALIZED (...)`). It is a single boolean,
// not two, because the two keywords were introduced together in every
// dialect that has either. It is an explicit method rather than bare
// interface presence so a dialect without the modifier is rejected with a
// typed ErrUnsupportedByDialect rather than rendered.
type CTEMaterializationDialect interface {
	Dialect
	SupportsCTEMaterialized() bool
}

// CTESearchCycleDialect is implemented by dialects that support the
// recursive-CTE `SEARCH` and `CYCLE` clauses:
//
//	WITH RECURSIVE name AS (<body>) SEARCH DEPTH FIRST BY <cols> SET <ordcol>
//	                                [CYCLE <cols> SET <is_cycle> USING <path>]
//
// A request on a dialect without the clauses is rejected with a typed
// ErrUnsupportedByDialect instead of invalid SQL.
type CTESearchCycleDialect interface {
	Dialect
	SupportsCTESearchCycle() bool
}

// ErrUnsupportedByDialect is the typed error query methods return when the
// resolved dialect lacks a capability the query needs. Callers test with
// errors.Is.
var ErrUnsupportedByDialect = errors.New("orm/dialect: unsupported by dialect")
