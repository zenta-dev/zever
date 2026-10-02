// Package postgres (this file) holds the DB-backed-adapter capabilities:
// pgvector vector operations, full-text search flavor, and row-lease
// compare-and-set claims.
package postgres

// SupportsVectorOps reports that Postgres supports the pgvector extension
// bundle: the `vector` type, `<=>` cosine-distance ordering, and the
// ivfflat/HNSW index methods.
func (Dialect) SupportsVectorOps() bool { return true }

// SupportsTSVector reports that Postgres supports `tsvector` full-text
// search (`to_tsvector` ranking over a GIN index).
func (Dialect) SupportsTSVector() bool { return true }

// SupportsFTS5 reports that Postgres supports SQLite's FTS5 virtual tables.
// It does not, so this always reports false.
func (Dialect) SupportsFTS5() bool { return false }

// SupportsLeaseClaim reports that Postgres supports row-lease
// compare-and-set claims: the atomic single-row
// `UPDATE ... WHERE lease_owner = ... AND lease_expires_at = ...` the
// queue and workflow adapters build crash-recovery leases on.
func (Dialect) SupportsLeaseClaim() bool { return true }
