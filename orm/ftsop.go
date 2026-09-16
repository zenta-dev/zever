package orm

// FTSOp identifies a full-text-search expression applied to a text column:
// a MATCH predicate, or a ranking expression used in ORDER BY. The value is
// abstract -- "does the column match this query", "rank rows by how well
// they match this query" -- and orm/render turns it into the concrete
// per-dialect syntax: Postgres's tsvector machinery
// (`to_tsvector('english', col) @@ plainto_tsquery('english', ?)`,
// `ts_rank(...)`), SQLite's FTS5 MATCH operator, and MySQL's
// `MATCH (col, ...) AGAINST (? [mode])` (see 's
// capability-table row, and render/fts.go). The Postgres SQL expression
// shapes mirror search/postgres exactly -- the legacy full-text
// implementation whose storage/search logic this phase wraps rather than
// duplicates (search/postgres/postgres.go's matchWhere and
// buildSearchQueries).
type FTSOp int

// Supported full-text-search operators.
const (
	// FTSMatch is a text-match predicate: `to_tsvector('english', col) @@
	// plainto_tsquery('english', ?)` on Postgres (query text normalized to
	// ANDed terms), `col MATCH ?` on SQLite (FTS5's own query syntax), and
	// `MATCH (col, ...) AGAINST (? [mode])` on MySQL (the mode comes from
	// FTSExpr.Mode; the optional multi-column list from FTSExpr.Columns).
	FTSMatch FTSOp = iota
	// FTSMatchTSQuery is the boolean-query variant of FTSMatch:
	// `to_tsvector('english', col) @@ to_tsquery('english', ?)` on
	// Postgres, where the bound text is expected to be valid tsquery syntax
	// (terms joined by & | !, double-quoted phrases). Postgres-only; SQLite
	// has no equivalent because FTS5's MATCH already accepts boolean query
	// syntax natively (the sqlite package exposes FTSMatch only).
	FTSMatchTSQuery
	// FTSRank is a ranking expression for ORDER BY:
	// `ts_rank(to_tsvector('english', col), plainto_tsquery('english', ?))`
	// on Postgres. SQLite's FTS5 ranking needs no expression -- it exposes
	// the hidden rank column instead (see the orm/fts/sqlite package).
	FTSRank
)

// FTSExpr is the erased FTS expression an NFTS predicate node (or an FTS
// OrderTerm) carries: which operator, and the search text. The base column
// lives on the surrounding Node/OrderTerm (Table/Column), exactly as a
// plain binary predicate's column does. The search text is bound as a
// placeholder argument at render time -- never string-formatted into the
// SQL (the repo's placeholder-only rule).
//
// Mode and Columns are MySQL-only: Columns is the MATCH column list
// (`MATCH (a, b) AGAINST (...)`; empty falls back to the node's single
// Column), and Mode selects the AGAINST mode. The Postgres and SQLite
// renderers ignore both.
type FTSExpr struct {
	Op      FTSOp
	Query   string
	Mode    FTSMode
	Columns []string
}

// FTSMode selects the MySQL `AGAINST` mode. It has no meaning on Postgres or
// SQLite, whose query text carries its own boolean syntax; the field is
// ignored there. The zero value is natural-language mode.
type FTSMode int

// Supported MySQL `AGAINST` modes: natural language (the MySQL default),
// boolean (`IN BOOLEAN MODE`, the caller's query string uses MySQL's
// boolean operators), and query expansion (`WITH QUERY EXPANSION`, MySQL
// widens the search with related words).
const (
	FTSNaturalLanguage FTSMode = iota
	FTSBoolean
	FTSQueryExpansion
)

// NewFTSPredicate wraps an FTS expression over table.column into a typed
// Predicate. It is exported for the orm/fts/* dialect packages to build
// predicates from their typed helpers; ordinary callers use those
// packages' fluent surfaces, never this constructor directly.
func NewFTSPredicate[T any](table, column string, expr FTSExpr) Predicate[T] {
	return Predicate[T]{n: Node{Kind: NFTS, Table: table, Column: column, FTS: &expr}}
}

// NewFTSOrderTerm builds an OrderTerm ordering by an FTS ranking expression
// (Postgres ts_rank). column is the searched column (an AnyColumn[T] built
// from a schema-derived Column[T,V] via Col()); the term renders as
// `ORDER BY ts_rank(to_tsvector('english', <column>), plainto_tsquery('english', ?)) DESC`
// (or ASC when desc is false). It is exported for the orm/fts/postgres
// package; SQLite's FTS5 ranking needs no expression (its hidden rank
// column is ordered through a plain OrderTerm -- see orm/fts/sqlite).
func NewFTSOrderTerm[T any](column AnyColumn[T], expr FTSExpr, desc bool) OrderTerm[T] {
	return OrderTerm[T]{Column: column, Desc: desc, FTS: &expr}
}
