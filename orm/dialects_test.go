package orm

import (
	"os"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// on adapts one concrete dialect capability method to a no-arg probe for
// the matrix table below, e.g. on(sl, sqlite.Dialect.SupportsCTE).
func on[S any](d S, m func(S) bool) func() bool { return func() bool { return m(d) } }

// matrixRow is one "## Capability matrix" row: the exact doc cells plus
// probes against the two default dialects. A nil probe means the row is
// syntax, not a capability (exact cell match only).
type matrixRow struct {
	feature    string
	sqliteCell string
	pgCell     string
	sqlite     func() bool
	pg         func() bool
}

// TestDialectMatrixMatchesDoc pins orm/DIALECTS.md to the capability
// methods: every row names a known feature with its exact cells, no row
// names an unknown feature, and each "yes"/"no" cell matches what the
// default dialect actually reports. Edit the doc and the code together.
func TestDialectMatrixMatchesDoc(t *testing.T) {
	sl, pg := sqlite.New(), postgres.New()

	rows := []matrixRow{
		{"WITH (plain CTE)", "yes", "yes", on(sl, sqlite.Dialect.SupportsCTE), on(pg, postgres.Dialect.SupportsCTE)},
		{"WITH RECURSIVE", "yes", "yes", on(sl, sqlite.Dialect.SupportsRecursive), on(pg, postgres.Dialect.SupportsRecursive)},
		{"UNION / UNION ALL", "yes", "yes", nil, nil},
		{"INTERSECT / EXCEPT", "yes", "yes", on(sl, sqlite.Dialect.SupportsIntersectExcept), on(pg, postgres.Dialect.SupportsIntersectExcept)},
		{"RETURNING on DML", "yes", "yes", on(sl, sqlite.Dialect.SupportsReturning), on(pg, postgres.Dialect.SupportsReturning)},
		{"EXPLAIN", "yes", "yes", nil, nil},
		{"EXPLAIN ANALYZE", "no", "yes", on(sl, sqlite.Dialect.SupportsExplainAnalyze), on(pg, postgres.Dialect.SupportsExplainAnalyze)},
		{"MERGE", "no", "yes (15+)", on(sl, sqlite.Dialect.SupportsMerge), on(pg, postgres.Dialect.SupportsMerge)},
		{"JSON arrow-text extraction", "yes", "yes", nil, nil},
		{"json_each / json_tree TVF", "yes", "no", on(sl, sqlite.Dialect.SupportsJSONEach), on(pg, postgres.Dialect.SupportsJSONEach)},
		{"jsonb_array_elements set-returning", "no", "yes", on(sl, sqlite.Dialect.SupportsJSONSetReturning), on(pg, postgres.Dialect.SupportsJSONSetReturning)},
		{"RIGHT JOIN", "yes (3.39+)", "yes", on(sl, sqlite.Dialect.SupportsRightJoin), on(pg, postgres.Dialect.SupportsRightJoin)},
		{"FULL JOIN", "yes (3.39+)", "yes", on(sl, sqlite.Dialect.SupportsFullJoin), on(pg, postgres.Dialect.SupportsFullJoin)},
		{"UPDATE FROM join", "yes", "yes", on(sl, sqlite.Dialect.SupportsUpdateJoin), on(pg, postgres.Dialect.SupportsUpdateJoin)},
		{"DELETE USING join", "no", "yes", on(sl, sqlite.Dialect.SupportsDeleteJoin), on(pg, postgres.Dialect.SupportsDeleteJoin)},
		{"LEFT JOIN inside UPDATE/DELETE", "no", "no", on(sl, sqlite.Dialect.SupportsLeftMutateJoin), on(pg, postgres.Dialect.SupportsLeftMutateJoin)},
		{"UPDATE with ORDER BY/LIMIT", "no", "no", on(sl, sqlite.Dialect.SupportsUpdateOrderLimit), on(pg, postgres.Dialect.SupportsUpdateOrderLimit)},
		{"DELETE with ORDER BY/LIMIT", "no", "no", on(sl, sqlite.Dialect.SupportsDeleteOrderLimit), on(pg, postgres.Dialect.SupportsDeleteOrderLimit)},
		{"UPDATE/DELETE with OFFSET", "no", "no", on(sl, sqlite.Dialect.SupportsMutateOffset), on(pg, postgres.Dialect.SupportsMutateOffset)},
		{"Joined UPDATE/DELETE with ORDER BY/LIMIT", "no", "no", on(sl, sqlite.Dialect.SupportsJoinedMutateOrderLimit), on(pg, postgres.Dialect.SupportsJoinedMutateOrderLimit)},
		{"ON CONFLICT target WHERE", "yes", "yes", on(sl, sqlite.Dialect.SupportsConflictTargetWhere), on(pg, postgres.Dialect.SupportsConflictTargetWhere)},
		{"ON CONFLICT DO UPDATE WHERE", "yes", "yes", on(sl, sqlite.Dialect.SupportsConflictUpdateWhere), on(pg, postgres.Dialect.SupportsConflictUpdateWhere)},
		{"SELECT FOR UPDATE", "no", "yes", on(sl, sqlite.Dialect.SupportsForUpdate), on(pg, postgres.Dialect.SupportsForUpdate)},
		{"SELECT FOR SHARE", "no", "yes", on(sl, sqlite.Dialect.SupportsForShare), on(pg, postgres.Dialect.SupportsForShare)},
		{"NOWAIT suffix", "no", "yes", on(sl, sqlite.Dialect.SupportsNoWait), on(pg, postgres.Dialect.SupportsNoWait)},
		{"SKIP LOCKED suffix", "no", "yes", on(sl, sqlite.Dialect.SupportsSkipLocked), on(pg, postgres.Dialect.SupportsSkipLocked)},
		{"FOR NO KEY UPDATE", "no", "yes", on(sl, sqlite.Dialect.SupportsForNoKeyUpdate), on(pg, postgres.Dialect.SupportsForNoKeyUpdate)},
		{"FOR KEY SHARE", "no", "yes", on(sl, sqlite.Dialect.SupportsForKeyShare), on(pg, postgres.Dialect.SupportsForKeyShare)},
		{"FOR UPDATE OF table list", "no", "yes", on(sl, sqlite.Dialect.SupportsForUpdateOf), on(pg, postgres.Dialect.SupportsForUpdateOf)},
		{"NULLS FIRST/LAST", "yes (3.30+)", "yes", on(sl, sqlite.Dialect.SupportsNullsOrdering), on(pg, postgres.Dialect.SupportsNullsOrdering)},
		{"DISTINCT ON", "no", "yes", on(sl, sqlite.Dialect.SupportsDistinctOn), on(pg, postgres.Dialect.SupportsDistinctOn)},
		{"TABLESAMPLE", "no", "yes", on(sl, sqlite.Dialect.SupportsTablesample), on(pg, postgres.Dialect.SupportsTablesample)},
		{"LATERAL derived-table join", "no", "yes", on(sl, sqlite.Dialect.SupportsLateral), on(pg, postgres.Dialect.SupportsLateral)},
		{"Aggregate FILTER clause", "yes (3.30+)", "yes", on(sl, sqlite.Dialect.SupportsAggregateFilter), on(pg, postgres.Dialect.SupportsAggregateFilter)},
		{"ROLLUP", "no", "yes", on(sl, sqlite.Dialect.SupportsRollup), on(pg, postgres.Dialect.SupportsRollup)},
		{"CUBE", "no", "yes", on(sl, sqlite.Dialect.SupportsCube), on(pg, postgres.Dialect.SupportsCube)},
		{"GROUPING SETS", "no", "yes", on(sl, sqlite.Dialect.SupportsGroupingSets), on(pg, postgres.Dialect.SupportsGroupingSets)},
		{"array_agg", "no", "yes", on(sl, sqlite.Dialect.SupportsArrayAgg), on(pg, postgres.Dialect.SupportsArrayAgg)},
		{"string_agg", "no", "yes", on(sl, sqlite.Dialect.SupportsStringAgg), on(pg, postgres.Dialect.SupportsStringAgg)},
		{"group_concat", "yes", "no", on(sl, sqlite.Dialect.SupportsGroupConcat), on(pg, postgres.Dialect.SupportsGroupConcat)},
		{"In-aggregate ORDER BY", "yes (3.44+)", "yes", on(sl, sqlite.Dialect.SupportsOrderedAggregates), on(pg, postgres.Dialect.SupportsOrderedAggregates)},
		{"ANY/ALL array predicates", "no", "yes", on(sl, sqlite.Dialect.SupportsArrayPredicates), on(pg, postgres.Dialect.SupportsArrayPredicates)},
		{"Window ROWS frame", "yes (3.28+)", "yes", on(sl, sqlite.Dialect.SupportsWindowFrameRows), on(pg, postgres.Dialect.SupportsWindowFrameRows)},
		{"Window RANGE frame", "yes (3.28+)", "yes", on(sl, sqlite.Dialect.SupportsWindowFrameRange), on(pg, postgres.Dialect.SupportsWindowFrameRange)},
		{"Window GROUPS frame", "yes (3.28+)", "yes", on(sl, sqlite.Dialect.SupportsWindowFrameGroups), on(pg, postgres.Dialect.SupportsWindowFrameGroups)},
		{"CTE MATERIALIZED modifier", "yes (3.35+)", "yes (12+)", on(sl, sqlite.Dialect.SupportsCTEMaterialized), on(pg, postgres.Dialect.SupportsCTEMaterialized)},
		{"CTE SEARCH/CYCLE clauses", "no", "yes (14+)", on(sl, sqlite.Dialect.SupportsCTESearchCycle), on(pg, postgres.Dialect.SupportsCTESearchCycle)},
		{"Vector ops (pgvector)", "no", "yes", on(sl, sqlite.Dialect.SupportsVectorOps), on(pg, postgres.Dialect.SupportsVectorOps)},
		{"tsvector full-text", "no", "yes", on(sl, sqlite.Dialect.SupportsTSVector), on(pg, postgres.Dialect.SupportsTSVector)},
		{"FTS5 full-text", "yes", "no", on(sl, sqlite.Dialect.SupportsFTS5), on(pg, postgres.Dialect.SupportsFTS5)},
		{"Lease-claim CAS", "yes", "yes", on(sl, sqlite.Dialect.SupportsLeaseClaim), on(pg, postgres.Dialect.SupportsLeaseClaim)},
		{"Placeholders", "?", "$n", nil, nil},
		{"Identifier quoting", `"..."`, `"..."`, nil, nil},
		{"Max bind params", "32766 (999 pre-3.32)", "65535", nil, nil},
	}

	raw, err := os.ReadFile("DIALECTS.md")
	if err != nil {
		t.Fatalf("read DIALECTS.md: %v", err)
	}

	doc := parseDialectMatrix(t, string(raw))

	if len(doc) != len(rows) {
		t.Errorf("DIALECTS.md has %d matrix rows, matrixRows has %d", len(doc), len(rows))
	}

	wantSet := make(map[string]struct{}, len(rows))
	for _, want := range rows {
		wantSet[want.feature] = struct{}{}
	}

	for _, want := range rows {
		got, ok := doc[want.feature]
		if !ok {
			t.Errorf("feature %q missing from DIALECTS.md matrix", want.feature)
			continue
		}

		if got[0] != want.sqliteCell {
			t.Errorf("feature %q sqlite cell = %q, want %q", want.feature, got[0], want.sqliteCell)
		}

		if got[1] != want.pgCell {
			t.Errorf("feature %q postgres cell = %q, want %q", want.feature, got[1], want.pgCell)
		}

		for _, tc := range []struct {
			name  string
			cell  string
			probe func() bool
		}{
			{"sqlite", want.sqliteCell, want.sqlite},
			{"postgres", want.pgCell, want.pg},
		} {
			if tc.probe == nil {
				continue
			}

			base := strings.SplitN(tc.cell, " ", 2)[0]
			wantBool := base == "yes"

			if !wantBool && base != "no" {
				t.Errorf("feature %q %s cell = %q, want a yes/no cell", want.feature, tc.name, tc.cell)
				continue
			}

			if gotBool := tc.probe(); gotBool != wantBool {
				t.Errorf("feature %q %s probe = %v, doc cell = %q", want.feature, tc.name, gotBool, tc.cell)
			}
		}
	}

	for feature := range doc {
		if _, ok := wantSet[feature]; !ok {
			t.Errorf("DIALECTS.md matrix names unknown feature %q", feature)
		}
	}

	// The JSON arrow-text row is ungated by design: neither in-tree dialect
	// implements dialect.JSONDialect, so renderers always allow `->>`. If
	// either gains the interface, the doc row and this assertion change
	// together.
	if _, ok := any(sl).(dialect.JSONDialect); ok {
		t.Error("sqlite implements dialect.JSONDialect, but DIALECTS.md documents the ->> fallback as ungated")
	}

	if _, ok := any(pg).(dialect.JSONDialect); ok {
		t.Error("postgres implements dialect.JSONDialect, but DIALECTS.md documents the ->> fallback as ungated")
	}

	// Syntax rows pinned to behavior: placeholder spelling and budgets.
	if got := sl.Placeholder(3); got != "?" {
		t.Errorf("sqlite.Placeholder(3) = %q, want %q", got, "?")
	}

	if got := pg.Placeholder(3); got != "$3" {
		t.Errorf("postgres.Placeholder(3) = %q, want %q", got, "$3")
	}

	if got := dialect.MaxParams(sl); got != 32766 {
		t.Errorf("MaxParams(sqlite) = %d, want 32766", got)
	}

	if got := dialect.MaxParams(pg); got != 65535 {
		t.Errorf("MaxParams(postgres) = %d, want 65535", got)
	}
}

// parseDialectMatrix reads the "| Capability | sqlite | postgres | ...|"
// markdown table under "## Capability matrix" into feature -> [sqlite,
// postgres] cells.
func parseDialectMatrix(t *testing.T, raw string) map[string][2]string {
	t.Helper()

	out := map[string][2]string{}
	lines := strings.Split(raw, "\n")
	inTable := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inTable {
			if trimmed == "## Capability matrix" {
				inTable = true
			}

			continue
		}
		if trimmed == "" {
			continue
		}

		if !strings.HasPrefix(trimmed, "|") {
			break
		}

		if strings.Contains(trimmed, "---") {
			continue
		}

		cells := strings.Split(trimmed, "|")
		if len(cells) < 4 {
			continue
		}

		feature := strings.TrimSpace(cells[1])
		if feature == "Capability" {
			continue
		}

		out[feature] = [2]string{strings.TrimSpace(cells[2]), strings.TrimSpace(cells[3])}
	}

	if len(out) == 0 {
		t.Fatal("no capability rows parsed from DIALECTS.md matrix")
	}

	return out
}
