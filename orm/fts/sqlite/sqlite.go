// Package sqlite provides typed helpers over SQLite FTS5 virtual tables
// (full-text search), built as orm predicates and order terms that compose
// into orm.From(...).Where(...) / OrderBy(...) exactly like Column methods
// do:
//
//	sqlite.Match(docBody, "distributed") // "col" MATCH ?
//	orm.From(docs).Where(sqlite.Match(docBody, "go")).
//		OrderBy(sqlite.Rank(docBody)) // ORDER BY "rank" DESC
//
// FTS5 is a virtual-table engine: the searched text must live in an FTS5
// virtual table (`CREATE VIRTUAL TABLE docs USING fts5(id, title, body)`,
// created via migration or raw DDL -- the FTS5 table shape is not
// expressible in the schema DSL), and orm queries over that table render an
// ordinary `SELECT ... FROM "docs"`. The MATCH predicate works against any
// indexed column of the virtual table; the query text uses FTS5's native
// boolean syntax (terms, `AND`/`OR`/`NOT`, `"phrases"`, prefix `term*`).
//
// The abstract full-text operator the helpers build renders as FTS5 MATCH
// on SQLite, and as Postgres tsvector machinery on Postgres (or MySQL
// `MATCH ... AGAINST` on MySQL) -- see orm/render/fts.go; the orm/fts/*
// packages are parallel surfaces over that same abstraction. The query text
// is ALWAYS a bound placeholder argument (the repo's placeholder-only rule),
// never string-formatted.
package sqlite

import (
	orm "github.com/zenta-dev/zever/orm"
)

// Match builds an FTS5 MATCH predicate over an indexed column c of an FTS5
// virtual table, rendered as `"col" MATCH ?`.
func Match[T any, V any](c orm.Column[T, V], text string) orm.Predicate[T] {
	return orm.NewFTSPredicate[T](c.Table(), c.Name(), orm.FTSExpr{Op: orm.FTSMatch, Query: text})
}

// Rank builds an ORDER BY term ordering rows by FTS5 relevance, rendered
// as `ORDER BY "rank" DESC` over the FTS5 virtual table's hidden rank
// column -- FTS5's standard ranking idiom (the rank column is "bm25
// relevance" in recent FTS5 versions; SQLite guarantees it is higher for
// better matches).
//
// FTS5 only defines the rank column for a query that has a MATCH
// constraint, so Rank is only meaningful in a query whose WHERE includes a
// Match (SQLite rejects `ORDER BY rank` without one). The argument's
// specific column is not referenced by the ordering -- rank scores the
// whole row across all of the table's indexed columns -- but passing it
// pins the entity type T so the term types as OrderTerm[T]. Unlike
// Postgres (orm/fts/postgres's Rank), FTS5 needs no rank expression: the
// hidden column is ordered through a plain OrderTerm, and the raw numeric
// score is equally unprojectable by the current orm Query API (see
// orm/fts/postgres's Rank doc comment for the shared projection limitation).
func Rank[T any, V any](_ orm.Column[T, V]) orm.OrderTerm[T] {
	// The rank column is not a schema column of T, so build its AnyColumn
	// ref through NewColumn -- the documented codegen-only string->column
	// bridge. The identifier is a compile-time constant here, never caller
	// input, so the OrderTerm stays injection-safe.
	return orm.OrderTerm[T]{Column: orm.NewColumn[T, int64]("", "rank").Col(), Desc: true}
}
