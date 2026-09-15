// Package sqlite provides an embedded full-text search backend backed by
// SQLite FTS5 without network calls.
//
// Documents are dual-written to a content table and an FTS5 index inside one
// transaction. Scores use -bm25 units (bm25 ranks lower-better, so negation
// orders higher-first). Unlike the meilisearch backend, sqlite is
// single-tenant and embedded: searches without an index filter span all
// indexes, and deletes remove every document sharing an id across indexes.
package sqlite
