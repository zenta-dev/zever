package orm

import (
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestMergeDialectTruthTable pins the MERGE capability matrix: Postgres 15+
// only (via its version-gated SupportsMerge); SQLite never reports support.
// Both implement dialect.MergeDialect, so the gate is the per-dialect
// boolean rather than interface presence.
func TestMergeDialectTruthTable(t *testing.T) {
	var (
		_ dialect.MergeDialect = sqlite.New()
		_ dialect.MergeDialect = postgres.New()
	)

	if !postgres.New().SupportsMerge() {
		t.Fatal("postgres.New().SupportsMerge() = false, want true")
	}

	if sqlite.New().SupportsMerge() {
		t.Fatal("sqlite.New().SupportsMerge() = true, want false (no MERGE at any version)")
	}
}
