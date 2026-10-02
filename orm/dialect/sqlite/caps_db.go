// Package sqlite (this file) holds the DB-backed-adapter capabilities:
// vector operations, full-text search flavor, and row-lease
// compare-and-set claims. SQLite has no pgvector extension and no tsvector,
// so those report false; its FTS5 virtual tables and single-row CAS updates
// report true.
package sqlite

// SupportsVectorOps reports that SQLite supports the pgvector extension
// bundle. No SQLite version has the `vector` type, `<=>` distance ordering,
// or ivfflat/HNSW indexes (the vectorstore sqlite leg brute-forces cosine
// distance in Go instead), so this always reports false.
func (Dialect) SupportsVectorOps() bool { return false }

// SupportsTSVector reports that SQLite supports `tsvector` full-text
// search. It has no tsvector type, so this always reports false.
func (Dialect) SupportsTSVector() bool { return false }

// SupportsFTS5 reports that SQLite supports FTS5 virtual tables with
// `MATCH` ranking.
func (Dialect) SupportsFTS5() bool { return true }

// SupportsLeaseClaim reports that SQLite supports row-lease
// compare-and-set claims: the atomic single-row
// `UPDATE ... WHERE lease_owner = ... AND lease_expires_at = ...` the
// queue and workflow adapters build crash-recovery leases on.
func (Dialect) SupportsLeaseClaim() bool { return true }
