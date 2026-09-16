// Package postgres (this file) holds the query-modifier capabilities added
// after the initial dialect surface: DISTINCT ON, the Postgres-only lock
// modes, and TABLESAMPLE.
package postgres

// SupportsDistinctOn reports that Postgres supports `SELECT DISTINCT ON
// (cols)`, a native Postgres extension.
func (Dialect) SupportsDistinctOn() bool { return true }

// SupportsForNoKeyUpdate reports that Postgres supports the `FOR NO KEY
// UPDATE` row-lock strength.
func (Dialect) SupportsForNoKeyUpdate() bool { return true }

// SupportsForKeyShare reports that Postgres supports the `FOR KEY SHARE`
// row-lock strength.
func (Dialect) SupportsForKeyShare() bool { return true }

// SupportsForUpdateOf reports that Postgres supports the `FOR ... OF <table>`
// target list.
func (Dialect) SupportsForUpdateOf() bool { return true }

// SupportsTablesample reports that Postgres supports the `TABLESAMPLE
// <method> (<percentage>)` FROM-clause extension.
func (Dialect) SupportsTablesample() bool { return true }
