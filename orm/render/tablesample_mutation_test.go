package render

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
)

// TestSelectModsTablesampleHundredValid pins the inclusive upper boundary of
// the TABLESAMPLE percentage range: 0 and 100 are valid and render verbatim.
// The suite rejected -1 and 101 but never accepted 100, so tightening the
// check to `>= 100` survived; this test kills that mutant.
func TestSelectModsTablesampleHundredValid(t *testing.T) {
	t.Parallel()

	for _, arg := range []float64{0, 100} {
		got, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
			SelectModifiers{Tablesample: Tablesample{Method: "SYSTEM", Arg: arg}})
		if err != nil {
			t.Fatalf("Arg %v err = %v, want valid", arg, err)
		}

		if !strings.Contains(got, "TABLESAMPLE SYSTEM") {
			t.Fatalf("Arg %v query = %q, want TABLESAMPLE SYSTEM clause", arg, got)
		}
	}

	got, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
		SelectModifiers{Tablesample: Tablesample{Method: "SYSTEM", Arg: 100}})
	if err != nil {
		t.Fatalf("SelectMods: %v", err)
	}

	want := `SELECT "id" FROM "widgets" TABLESAMPLE SYSTEM (100)`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

// TestSelectModsLockNoWaitWinsOverSkipLocked pins the documented precedence
// when both row-lock modifiers are set: NOWAIT wins (the public API calls
// them mutually exclusive with "last one set wins", and the renderer checks
// NoWait first). Swapping the check order survived the suite; this test kills
// that mutant.
func TestSelectModsLockNoWaitWinsOverSkipLocked(t *testing.T) {
	t.Parallel()

	got, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
		SelectModifiers{Lock: LockForUpdate, NoWait: true, SkipLocked: true})
	if err != nil {
		t.Fatalf("SelectMods: %v", err)
	}

	want := `SELECT "id" FROM "widgets" FOR UPDATE NOWAIT`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

// TestSelectModsTablesampleLowercaseNormalized proves a lowercase sampling
// method renders upper-cased: validation accepts it case-insensitively, so
// the clause must normalize rather than interpolate caller text. Dropping
// the normalization survived the suite (tests only used uppercase); this
// test kills that mutant.
func TestSelectModsTablesampleLowercaseNormalized(t *testing.T) {
	t.Parallel()

	got, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
		SelectModifiers{Tablesample: Tablesample{Method: "bernoulli", Arg: 5.5}})
	if err != nil {
		t.Fatalf("SelectMods: %v", err)
	}

	want := `SELECT "id" FROM "widgets" TABLESAMPLE BERNOULLI (5.5)`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}
