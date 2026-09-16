// Package postgres (this file) holds the table-valued JSON source
// capabilities. Postgres supports set-returning JSON functions in a FROM
// clause, and has no json1 `json_each`/`json_tree` functions.
package postgres

// SupportsJSONEach reports that Postgres supports the json1
// `json_each`/`json_tree` table-valued functions. Postgres has no such
// functions, so this always reports false.
func (Dialect) SupportsJSONEach() bool { return false }

// SupportsJSONSetReturning reports that Postgres supports a set-returning
// JSON function as a FROM-clause source, correlated implicitly to earlier
// FROM entries. `jsonb_array_elements` has existed since Postgres 9.3's
// JSON support and needs no version gate on any version this package
// supports.
func (Dialect) SupportsJSONSetReturning() bool { return true }
