//go:build ormcompare

package bench

import "testing"

// TODO(verify): GORM comparison for the bench query set. Enabling it
// needs gorm.io/gorm plus a pure-Go sqlite driver (e.g.
// github.com/glebarez/sqlite over the already-pinned modernc.org/sqlite)
// added to orm/go.mod with a written justification, then one benchmark
// per query shape in bench_test.go mirroring the fixed set (point lookup,
// filtered ORDER BY + LIMIT, count, insert) against the same seeded
// :memory: database. This stub exists so the shape of the comparison is
// committed; it runs nothing until the dependency lands.
func TestGormCompareNotWired(t *testing.T) {
	t.Skip("TODO(verify): GORM comparison needs its sqlite-backed dependency wired under the ormcompare tag")
}
