package render

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
)

// TestSelectModsDistinctOn pins the exact text DISTINCT ON renders, before
// the projection, and its interaction with ORDER BY.
func TestSelectModsDistinctOn(t *testing.T) {
	t.Parallel()
	got, args, err := SelectMods(
		postgres.New(), "widgets", []string{"id", "name"},
		Node{}, []OrderTerm{{Column: "id"}}, 0, 0,
		SelectModifiers{DistinctOn: []string{"id"}},
	)
	if err != nil {
		t.Fatalf("SelectMods: %v", err)
	}

	want := `SELECT DISTINCT ON ("id") "id", "name" FROM "widgets" ORDER BY "id" ASC`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

// TestSelectModsDistinctOnMultiCol proves the ON list renders every column,
// quoted and comma-separated.
func TestSelectModsDistinctOnMultiCol(t *testing.T) {
	t.Parallel()
	got, _, err := SelectMods(
		postgres.New(), "widgets", []string{"id", "name", "quantity"},
		Node{}, nil, 0, 0,
		SelectModifiers{DistinctOn: []string{"name", "quantity"}},
	)
	if err != nil {
		t.Fatalf("SelectMods: %v", err)
	}

	want := `SELECT DISTINCT ON ("name", "quantity") "id", "name", "quantity" FROM "widgets"`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

// TestSelectModsDistinctOnEmptyErrors proves an explicitly empty ON list is a
// rendering error rather than an invalid `DISTINCT ON ()`.
func TestSelectModsDistinctOnEmptyErrors(t *testing.T) {
	t.Parallel()
	if _, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
		SelectModifiers{DistinctOn: []string{}}); err == nil {
		t.Fatalf("SelectMods(DistinctOn=[]) err = nil, want an error")
	}
}

// TestSelectModsDistinctOnWithLockErrors proves DISTINCT ON participates in
// the DISTINCT + row-lock guard.
func TestSelectModsDistinctOnWithLockErrors(t *testing.T) {
	t.Parallel()
	_, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
		SelectModifiers{DistinctOn: []string{"id"}, Lock: LockForUpdate})
	if err == nil || !strings.Contains(err.Error(), "DISTINCT") {
		t.Fatalf("err = %v, want a DISTINCT + lock error", err)
	}
}

// TestSelectModsDistinctOnCapabilityGate proves the renderer rejects
// DISTINCT ON on a dialect that lacks DistinctOnDialect (SQLite) with the
// typed dialect.ErrUnsupportedByDialect.
func TestSelectModsDistinctOnCapabilityGate(t *testing.T) {
	t.Parallel()
	_, _, err := SelectMods(mockNoCap{}, "widgets", []string{"id"}, Node{}, nil, 0, 0,
		SelectModifiers{DistinctOn: []string{"id"}})
	if err == nil {
		t.Fatalf("err = nil, want an unsupported-by-dialect error")
	}
}

// mockNoCap is a base-only dialect (no capability sub-interfaces) used by the
// capability-gate render tests.
type mockNoCap struct{}

func (mockNoCap) Name() string               { return "mock-nocap" }
func (mockNoCap) Placeholder(int) string     { return "?" }
func (mockNoCap) QuoteIdent(s string) string { return `"` + s + `"` }

// TestSelectModsDistinctOnShapeCacheSeparates proves the ON list is part of
// the shape-cache key.
func TestSelectModsDistinctOnShapeCacheSeparates(t *testing.T) {
	d := postgres.New()

	renderTwice(t, func() (string, []any, error) {
		return SelectMods(d, "widgets", []string{"id", "name"}, Node{}, nil, 0, 0, SelectModifiers{DistinctOn: []string{"id"}})
	})

	renderTwice(t, func() (string, []any, error) {
		return SelectMods(d, "widgets", []string{"id", "name"}, Node{}, nil, 0, 0, SelectModifiers{DistinctOn: []string{"name"}})
	})

	resetShapeCache()

	a, _, _ := SelectMods(d, "widgets", []string{"id", "name"}, Node{}, nil, 0, 0, SelectModifiers{DistinctOn: []string{"id"}})
	b, _, _ := SelectMods(d, "widgets", []string{"id", "name"}, Node{}, nil, 0, 0, SelectModifiers{DistinctOn: []string{"name"}})

	if a == b {
		t.Fatalf("distinct-on shapes collided: %q", a)
	}
}
