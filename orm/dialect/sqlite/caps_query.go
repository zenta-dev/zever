// Package sqlite (this file) holds the query-modifier capabilities added
// after the initial dialect surface. SQLite has no DISTINCT ON, no
// row-level locking, and no TABLESAMPLE, so every method here reports false.
package sqlite

// SupportsDistinctOn reports that SQLite supports `SELECT DISTINCT ON
// (cols)`. It has no such syntax, so this always reports false.
func (Dialect) SupportsDistinctOn() bool { return false }

// SupportsForNoKeyUpdate reports that SQLite supports `FOR NO KEY UPDATE`.
// It has no row-level locking at all, so this always reports false.
func (Dialect) SupportsForNoKeyUpdate() bool { return false }

// SupportsForKeyShare reports that SQLite supports `FOR KEY SHARE`. It has
// no row-level locking at all, so this always reports false.
func (Dialect) SupportsForKeyShare() bool { return false }

// SupportsForUpdateOf reports that SQLite supports the `FOR ... OF <table>`
// target list. It has no row-level locking at all, so this always reports
// false.
func (Dialect) SupportsForUpdateOf() bool { return false }

// SupportsTablesample reports that SQLite supports `TABLESAMPLE`. It has no
// such clause, so this always reports false.
func (Dialect) SupportsTablesample() bool { return false }
