//go:build ormcompare

package bench

import "testing"

// TODO(verify): sqlc comparison for the bench query set. sqlc is a code
// generator, not a runtime library, so a fair comparison needs its
// generated query code checked in (generated with `sqlc generate` from a
// fixed schema + queries.sql covering the same four shapes: point lookup,
// filtered ORDER BY + LIMIT, count, insert) and benchmarked against the
// same seeded :memory: database. Hand-writing "sqlc-equivalent" SQL
// would misrepresent the tool; the Raw* benchmarks in bench_test.go are
// the honest hand-written-SQL baseline until generated code lands. This
// stub exists so the shape of the comparison is committed; it runs
// nothing until that code is checked in.
func TestSQLCCompareNotWired(t *testing.T) {
	t.Skip("TODO(verify): sqlc comparison needs sqlc-generated query code checked in under the ormcompare tag")
}
