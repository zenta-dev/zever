package render

import (
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestRenderEdgeDeterminism pins that rendering the same shape twice yields
// byte-identical output (shape-cache keying must not reorder anything).
func TestRenderEdgeDeterminism(t *testing.T) {
	t.Parallel()

	d := sqlite.New()
	cols := []string{"id", "name", "quantity"}
	order := []OrderTerm{{Column: "id"}}

	first, _, err := Select(d, "widgets", cols, Node{}, order, 0, 0)
	if err != nil {
		t.Fatalf("Select err = %v", err)
	}

	for range 10 {
		got, _, err := Select(d, "widgets", cols, Node{}, order, 0, 0)
		if err != nil {
			t.Fatalf("Select err = %v", err)
		}
		if got != first {
			t.Fatalf("Select not deterministic: got %q, want %q", got, first)
		}
	}
}

// TestSelect_nilColumns covers the boundary where no projection is supplied.
func TestSelect_nilColumns(t *testing.T) {
	t.Parallel()

	if _, _, err := Select(sqlite.New(), "widgets", nil, Node{}, nil, 0, 0); err != nil {
		t.Fatalf("Select(nil cols) err = %v, want nil", err)
	}
}

// TestGroupedSelect_emptyGroups pins the fallback to COUNT(*) when neither
// group terms nor aggregates are supplied.
func TestGroupedSelect_emptyGroups(t *testing.T) {
	t.Parallel()

	q, _, err := GroupedSelect(sqlite.New(), "orders", nil, nil, Node{}, HavingNode{})
	if err != nil {
		t.Fatalf("GroupedSelect err = %v, want nil", err)
	}

	const want = `SELECT COUNT(*) FROM "orders"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

// TestInsert_fewerValuesThanColumns pins the boundary where the value list is
// shorter than the column list: only the supplied values are bound.
func TestInsert_fewerValuesThanColumns(t *testing.T) {
	t.Parallel()

	_, args, err := Insert(sqlite.New(), "widgets", []string{"id", "name", "quantity"}, []any{"w1"})
	if err != nil {
		t.Fatalf("Insert err = %v, want nil", err)
	}
	if len(args) != 1 {
		t.Fatalf("args = %d, want 1", len(args))
	}
}

// TestUpdate_nilSets covers the boundary where no assignment is supplied.
func TestUpdate_nilSets(t *testing.T) {
	t.Parallel()

	if _, _, err := Update(sqlite.New(), "widgets", nil, Node{}, nil, 0, 0); err != nil {
		t.Fatalf("Update(nil sets) err = %v, want nil", err)
	}
}
