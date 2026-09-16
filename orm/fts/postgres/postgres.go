// Package postgres provides typed helpers over Postgres full-text search
// (tsvector/ts_rank), built as orm predicates and order terms that compose
// into orm.From(...).Where(...) / OrderBy(...) exactly like Column methods
// do:
//
//	postgres.Match(docBody, "distributed systems") // to_tsvector('english', col) @@ plainto_tsquery('english', ?)
//	postgres.MatchTSQuery(docBody, "sql & \"acid\"") // to_tsvector('english', col) @@ to_tsquery('english', ?)
//	orm.From(docs).Where(postgres.Match(docBody, "go")).
//		OrderBy(postgres.Rank(docBody, "go")) // ORDER BY ts_rank(to_tsvector('english', col), plainto_tsquery('english', ?)) DESC
//
// Every helper returns a orm.Predicate[T] (or a orm.OrderTerm[T]), so the
// whole surface composes with the existing query builders -- no raw SQL at
// the call site. The abstract full-text operator the helpers build renders
// as Postgres tsvector machinery on Postgres, and as FTS5 MATCH on SQLite
// (see orm/render/fts.go); the two orm/fts/* packages are parallel surfaces
// over that same abstraction.
//
// The SQL expression shapes -- `to_tsvector('english', col) @@
// plainto_tsquery('english', ?)` for matching, `ts_rank(...) AS score` for
// ranking -- are exactly the ones search/postgres already executes against
// its own search_documents table (search/postgres/postgres.go's matchWhere
// and buildSearchQueries): this package exposes those same expressions as
// typed, composable orm predicates/order terms against arbitrary user
// tables and columns, integrating with the existing full-text machinery
// rather than duplicating its storage/search logic. The 'english'
// text-search configuration is fixed, matching search/postgres. The query
// text is ALWAYS a bound placeholder argument (the repo's placeholder-only
// rule), never string-formatted.
//
// MySQL FTS lives in the parallel orm/fts/mysql package (`MATCH (cols…)
// AGAINST (? [mode])`); a predicate built by either package renders on the
// dialect whose query it executes on.
package postgres

import (
	orm "github.com/zenta-dev/zever/orm"
)

// Match builds a text-match predicate over c, rendered as
// `to_tsvector('english', "c") @@ plainto_tsquery('english', ?)`: does the
// column's document match the query? plainto_tsquery normalizes the bound
// text into ANDed terms, so no caller-side query syntax is needed.
func Match[T any, V any](c orm.Column[T, V], text string) orm.Predicate[T] {
	return orm.NewFTSPredicate[T](c.Table(), c.Name(), orm.FTSExpr{Op: orm.FTSMatch, Query: text})
}

// MatchTSQuery builds the boolean-query variant of Match, rendered as
// `to_tsvector('english', "c") @@ to_tsquery('english', ?)`: the bound text
// is expected to be valid tsquery syntax (terms joined by & | !, with
// double-quoted phrases), giving callers AND/OR/NOT/phrase control that
// plainto_tsquery's plain-text normalization does not offer.
func MatchTSQuery[T any, V any](c orm.Column[T, V], text string) orm.Predicate[T] {
	return orm.NewFTSPredicate[T](c.Table(), c.Name(), orm.FTSExpr{Op: orm.FTSMatchTSQuery, Query: text})
}

// Rank builds an ORDER BY term ranking rows by how well c's document
// matches text, rendered as
// `ORDER BY ts_rank(to_tsvector('english', "c"), plainto_tsquery('english', ?)) DESC`
// -- the same ts_rank expression search/postgres's buildSearchQueries
// projects as `AS score`. Descending is the default, since a search
// naturally wants the best match first.
//
// Projection limitation (documented): the current orm Query API has no way
// to SELECT an arbitrary expression, so the raw numeric score cannot be
// projected alongside the rows -- only ordered by. A caller who needs the
// score value itself can order by Rank and re-scan, or fall back to a raw
// query; this phase deliberately provides the ordering expression rather
// than inventing a new select mechanism.
func Rank[T any, V any](c orm.Column[T, V], text string) orm.OrderTerm[T] {
	return orm.NewFTSOrderTerm[T](c.Col(), orm.FTSExpr{Op: orm.FTSRank, Query: text}, true)
}
