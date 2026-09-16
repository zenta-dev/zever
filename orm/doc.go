// Package orm is a typed, fluent SQL query builder over code-generated
// table and column references.
//
// The entry point is From over a Table, chained with Where, OrderBy,
// Limit and Offset, and executed with All, First, FirstOrErr, Count,
// Exists or Stream against a db.DB. Predicates are built from typed
// Column and NullableColumn comparisons, composed with And, Or and Not,
// and ordered with Asc and Desc terms. Table, Column and NullableColumn
// values are constructed with NewTable, NewColumn and NewNullableColumn,
// which schema codegen calls with derived table and column names; the
// unexported fields keep hand-written code from forging column references
// from raw strings.
package orm
