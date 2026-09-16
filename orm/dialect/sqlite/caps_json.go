// Package sqlite (this file) holds the table-valued JSON source
// capabilities. SQLite ships the json1 `json_each`/`json_tree` functions, so
// it supports that family; it has no set-returning JSON functions, so it
// reports false for that family.
package sqlite

// SupportsJSONEach reports that SQLite supports the json1 table-valued
// functions `json_each`/`json_tree` in a FROM clause. json1 is part of the
// SQLite amalgamation at every supported version, so this is unconditional.
func (Dialect) SupportsJSONEach() bool { return true }

// SupportsJSONSetReturning reports that SQLite supports set-returning JSON
// functions in a FROM clause. SQLite has no such functions, so this always
// reports false.
func (Dialect) SupportsJSONSetReturning() bool { return false }
