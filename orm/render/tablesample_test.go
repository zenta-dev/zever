package render

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestSelectModsTablesample pins the exact TABLESAMPLE text and its
// placement between FROM and WHERE, with placeholder numbering unaffected.
func TestSelectModsTablesample(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindBinary, Table: "widgets", Column: "quantity", Op: OpGt, Value: int64(15)}

	got, args, err := SelectMods(
		postgres.New(), "widgets", []string{"id"},
		where, nil, 0, 0,
		SelectModifiers{Tablesample: Tablesample{Method: "SYSTEM", Arg: 10}},
	)
	if err != nil {
		t.Fatalf("SelectMods: %v", err)
	}

	want := `SELECT "id" FROM "widgets" TABLESAMPLE SYSTEM (10) WHERE "quantity" > $1`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}

	if len(args) != 1 || args[0] != int64(15) {
		t.Fatalf("args = %#v, want [15]", args)
	}

	got, _, err = SelectMods(
		postgres.New(), "widgets", []string{"id"},
		Node{}, nil, 0, 0,
		SelectModifiers{Tablesample: Tablesample{Method: "BERNOULLI", Arg: 5.5}},
	)
	if err != nil {
		t.Fatalf("SelectMods: %v", err)
	}

	want = `SELECT "id" FROM "widgets" TABLESAMPLE BERNOULLI (5.5)`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

// TestSelectModsTablesampleCapabilityGate proves a non-Postgres dialect
// rejects TABLESAMPLE with the typed dialect.ErrUnsupportedByDialect.
func TestSelectModsTablesampleCapabilityGate(t *testing.T) {
	t.Parallel()
	for _, d := range []dialect.Dialect{mockNoCap{}, sqlite.New()} {
		_, _, err := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0,
			SelectModifiers{Tablesample: Tablesample{Method: "SYSTEM", Arg: 10}})
		if err == nil || !strings.Contains(err.Error(), "TABLESAMPLE") {
			t.Fatalf("%s err = %v, want TABLESAMPLE unsupported error", d.Name(), err)
		}
	}
}

// TestSelectModsTablesampleInvalidMethod proves an unknown sampling method is
// rejected rather than interpolated verbatim into SQL.
func TestSelectModsTablesampleInvalidMethod(t *testing.T) {
	t.Parallel()
	_, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
		SelectModifiers{Tablesample: Tablesample{Method: "SYSTEM; DROP TABLE widgets", Arg: 10}})
	if err == nil {
		t.Fatalf("err = nil, want an invalid-method error")
	}
}

// TestSelectModsTablesampleShapeCache proves method and arg are part of the
// shape-cache key.
func TestSelectModsTablesampleShapeCache(t *testing.T) {
	d := postgres.New()

	renderTwice(t, func() (string, []any, error) {
		return SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Tablesample: Tablesample{Method: "SYSTEM", Arg: 10}})
	})

	resetShapeCache()

	sys, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Tablesample: Tablesample{Method: "SYSTEM", Arg: 10}})
	bern, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Tablesample: Tablesample{Method: "BERNOULLI", Arg: 10}})
	arg, _, _ := SelectMods(d, "widgets", []string{"id"}, Node{}, nil, 0, 0, SelectModifiers{Tablesample: Tablesample{Method: "SYSTEM", Arg: 20}})

	if sys == bern || sys == arg || bern == arg {
		t.Fatalf("tablesample shapes collided: sys=%q bern=%q arg=%q", sys, bern, arg)
	}
}

func TestSelectModsTablesamplePercentageRange(t *testing.T) {
	t.Parallel()

	for _, arg := range []float64{-1, 101} {
		_, _, err := SelectMods(postgres.New(), "widgets", []string{"id"}, Node{}, nil, 0, 0,
			SelectModifiers{Tablesample: Tablesample{Method: "SYSTEM", Arg: arg}})
		if err == nil {
			t.Fatalf("Arg %v: err = nil, want an out-of-range error", arg)
		}
	}
}
