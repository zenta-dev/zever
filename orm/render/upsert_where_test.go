package render

import (
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// whereBinary is a small helper building a single-column comparison Node for
// the partial-upsert-where render tests (an unqualified column, like the
// predicates orm's typed columns produce for a single-table statement).
func whereBinary(col string, op Op, value any) Node {
	return Node{Kind: KindBinary, Column: col, Op: op, Value: value}
}

// TestInsertOnConflictWhere pins the exact SQL and argument order for a
// single-row Postgres upsert carrying BOTH predicates: the partial-index
// target predicate (bound BEFORE the SET list, matching text order) and the
// conditional-update predicate (bound AFTER the SET list). Postgres `$N`
// numbering must follow that same order.
func TestInsertOnConflictWhere(t *testing.T) {
	t.Parallel()
	q, args, err := InsertOnConflictWhere(postgres.New(), "widgets",
		[]string{"id", "name", "quantity"}, []any{"w1", "Alpha", int64(10)},
		[]string{"id"},
		ConflictWhere{
			Target: whereBinary("active", OpEq, int64(1)),
			Update: whereBinary("quantity", OpLt, int64(100)),
		},
		[]Assignment{{Column: "quantity", Value: int64(99)}, {Column: "name", Value: "Beta"}},
		[]string{"id"})
	if err != nil {
		t.Fatalf("InsertOnConflictWhere: %v", err)
	}

	want := `INSERT INTO "widgets" ("id", "name", "quantity") VALUES ($1, $2, $3) ON CONFLICT ("id") WHERE "active" = $4 DO UPDATE SET "quantity" = $5, "name" = $6 WHERE "quantity" < $7 RETURNING "id"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	wantArgs := []any{"w1", "Alpha", int64(10), int64(1), int64(99), "Beta", int64(100)}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

// TestInsertOnConflictWhereTargetOnlyDoNothing pins the target predicate on
// a DO NOTHING upsert (there is no update predicate and no SET list).
func TestInsertOnConflictWhereTargetOnlyDoNothing(t *testing.T) {
	t.Parallel()
	q, args, err := InsertOnConflictWhere(postgres.New(), "widgets",
		[]string{"id", "name"}, []any{"w1", "Alpha"},
		[]string{"id"},
		ConflictWhere{Target: whereBinary("active", OpEq, int64(1))},
		nil, nil)
	if err != nil {
		t.Fatalf("InsertOnConflictWhere: %v", err)
	}

	want := `INSERT INTO "widgets" ("id", "name") VALUES ($1, $2) ON CONFLICT ("id") WHERE "active" = $3 DO NOTHING`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"w1", "Alpha", int64(1)}) {
		t.Fatalf("args = %#v", args)
	}
}

// TestInsertOnConflictWhereUpdateOnly proves the target predicate and the
// update predicate are independently optional: with only the update
// predicate, the WHERE lands after the SET list and its arg follows the SET
// args.
func TestInsertOnConflictWhereUpdateOnly(t *testing.T) {
	t.Parallel()
	q, args, err := InsertOnConflictWhere(postgres.New(), "widgets",
		[]string{"id", "name", "quantity"}, []any{"w1", "Alpha", int64(10)},
		[]string{"id"},
		ConflictWhere{Update: whereBinary("quantity", OpLt, int64(100))},
		[]Assignment{{Column: "name", Value: "Beta"}},
		nil)
	if err != nil {
		t.Fatalf("InsertOnConflictWhere: %v", err)
	}

	want := `INSERT INTO "widgets" ("id", "name", "quantity") VALUES ($1, $2, $3) ON CONFLICT ("id") DO UPDATE SET "name" = $4 WHERE "quantity" < $5`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"w1", "Alpha", int64(10), "Beta", int64(100)}) {
		t.Fatalf("args = %#v", args)
	}
}

// TestInsertManyOnConflictWhere pins the multi-row numbering: all row values
// first, then the target predicate, then the SET assignment, then the update
// predicate.
func TestInsertManyOnConflictWhere(t *testing.T) {
	t.Parallel()
	q, args, err := InsertManyOnConflictWhere(postgres.New(), "widgets",
		[]string{"id", "name"}, [][]any{{"w1", "Alpha"}, {"w2", "Beta"}},
		[]string{"id"},
		ConflictWhere{
			Target: whereBinary("active", OpEq, int64(1)),
			Update: whereBinary("quantity", OpLt, int64(100)),
		},
		[]Assignment{{Column: "quantity", Value: int64(5)}},
		nil)
	if err != nil {
		t.Fatalf("InsertManyOnConflictWhere: %v", err)
	}

	want := `INSERT INTO "widgets" ("id", "name") VALUES ($1, $2), ($3, $4) ON CONFLICT ("id") WHERE "active" = $5 DO UPDATE SET "quantity" = $6 WHERE "quantity" < $7`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	wantArgs := []any{"w1", "Alpha", "w2", "Beta", int64(1), int64(5), int64(100)}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

// TestInsertOnConflictWhereSQLite proves the SQLite rendering uses "?"
// placeholders with the same text and argument order as Postgres.
func TestInsertOnConflictWhereSQLite(t *testing.T) {
	t.Parallel()
	q, args, err := InsertOnConflictWhere(sqlite.New(), "widgets",
		[]string{"id", "name"}, []any{"w1", "Alpha"},
		[]string{"id"},
		ConflictWhere{
			Target: whereBinary("active", OpEq, int64(1)),
			Update: whereBinary("quantity", OpLt, int64(100)),
		},
		[]Assignment{{Column: "name", Value: "Beta"}},
		[]string{"id"})
	if err != nil {
		t.Fatalf("InsertOnConflictWhere: %v", err)
	}

	want := `INSERT INTO "widgets" ("id", "name") VALUES (?, ?) ON CONFLICT ("id") WHERE "active" = ? DO UPDATE SET "name" = ? WHERE "quantity" < ? RETURNING "id"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	wantArgs := []any{"w1", "Alpha", int64(1), "Beta", int64(100)}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

// TestInsertOnConflictWhereNoPredicates proves the *Where entry point renders
// byte-identically to the unpredicated InsertOnConflict (so the existing
// entry point can delegate to it without behavior drift).
func TestInsertOnConflictWhereNoPredicates(t *testing.T) {
	t.Parallel()
	withWhere, withArgs, err := InsertOnConflictWhere(postgres.New(), "widgets",
		[]string{"id"}, []any{"w1"}, []string{"id"},
		ConflictWhere{}, []Assignment{{Column: "quantity", Value: int64(1)}}, nil)
	if err != nil {
		t.Fatalf("InsertOnConflictWhere: %v", err)
	}

	want := `INSERT INTO "widgets" ("id") VALUES ($1) ON CONFLICT ("id") DO UPDATE SET "quantity" = $2`
	if withWhere != want {
		t.Fatalf("query = %q, want %q", withWhere, want)
	}

	if !reflect.DeepEqual(withArgs, []any{"w1", int64(1)}) {
		t.Fatalf("args = %#v", withArgs)
	}
}

// TestInsertSelectOnConflictWhere pins the INSERT ... SELECT composition:
// both predicates render AFTER the SELECT body, and the Postgres `$N`
// numbering continues from the SELECT's own bound arguments.
func TestInsertSelectOnConflictWhere(t *testing.T) {
	t.Parallel()
	src := SelectSource{
		Table:   "widgets_src",
		Columns: []string{"id", "name", "quantity"},
		Where:   whereBinary("quantity", OpGt, int64(15)),
		Limit:   5,
	}

	q, args, err := InsertSelectOnConflictWhere(postgres.New(), "widgets",
		[]string{"id", "name", "quantity"}, src, []string{"id"},
		ConflictWhere{
			Target: whereBinary("active", OpEq, int64(1)),
			Update: whereBinary("quantity", OpLt, int64(100)),
		},
		[]Assignment{{Column: "name", Value: "Merged"}},
		nil)
	if err != nil {
		t.Fatalf("InsertSelectOnConflictWhere: %v", err)
	}

	// SELECT binds $1 (quantity > 15) and $2 (LIMIT 5); the target predicate
	// is $3, the SET assignment $4, and the update predicate $5.
	want := `INSERT INTO "widgets" ("id", "name", "quantity") SELECT "id", "name", "quantity" FROM "widgets_src" WHERE "quantity" > $1 LIMIT $2 ON CONFLICT ("id") WHERE "active" = $3 DO UPDATE SET "name" = $4 WHERE "quantity" < $5`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	wantArgs := []any{int64(15), 5, int64(1), "Merged", int64(100)}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
}

func TestInsertOnConflictWhereErrorPaths(t *testing.T) {
	t.Parallel()

	badTarget := ConflictWhere{Target: Node{Kind: KindBinary, Column: "active", Op: OpEqAny, Value: 1}}
	badUpdate := ConflictWhere{Update: Node{Kind: KindBinary, Column: "quantity", Op: OpEqAny, Value: 1}}

	t.Run("target predicate error propagates", func(t *testing.T) {
		t.Parallel()

		_, _, err := InsertOnConflictWhere(postgres.New(), "widgets",
			[]string{"id"}, []any{"w1"}, []string{"id"}, badTarget,
			[]Assignment{{Column: "name", Value: "x"}}, nil)
		if err == nil {
			t.Fatal("err = nil, want the target WHERE error")
		}
	})

	t.Run("update predicate error propagates", func(t *testing.T) {
		t.Parallel()

		_, _, err := InsertOnConflictWhere(postgres.New(), "widgets",
			[]string{"id"}, []any{"w1"}, []string{"id"}, badUpdate,
			[]Assignment{{Column: "name", Value: "x"}}, nil)
		if err == nil {
			t.Fatal("err = nil, want the update WHERE error")
		}
	})

	t.Run("many target predicate error propagates", func(t *testing.T) {
		t.Parallel()

		_, _, err := InsertManyOnConflictWhere(postgres.New(), "widgets",
			[]string{"id"}, [][]any{{"w1"}}, []string{"id"}, badTarget,
			[]Assignment{{Column: "name", Value: "x"}}, nil)
		if err == nil {
			t.Fatal("err = nil, want the target WHERE error")
		}
	})

	t.Run("many update predicate error propagates", func(t *testing.T) {
		t.Parallel()

		_, _, err := InsertManyOnConflictWhere(postgres.New(), "widgets",
			[]string{"id"}, [][]any{{"w1"}}, []string{"id"}, badUpdate,
			[]Assignment{{Column: "name", Value: "x"}}, nil)
		if err == nil {
			t.Fatal("err = nil, want the update WHERE error")
		}
	})

	t.Run("short first row truncates columns", func(t *testing.T) {
		t.Parallel()

		q, args := InsertOnConflict(postgres.New(), "widgets",
			[]string{"id", "name"}, []any{"w1"}, []string{"id"}, nil, nil)

		want := `INSERT INTO "widgets" ("id") VALUES ($1) ON CONFLICT ("id") DO NOTHING`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"w1"}) {
			t.Fatalf("args = %#v, want [w1]", args)
		}
	})

	t.Run("short later row truncates placeholders", func(t *testing.T) {
		t.Parallel()

		q, args := InsertManyOnConflict(postgres.New(), "widgets",
			[]string{"id", "name"}, [][]any{{"w1", "Alpha"}, {"w2"}}, []string{"id"}, nil, nil)

		want := `INSERT INTO "widgets" ("id", "name") VALUES ($1, $2), ($3) ON CONFLICT ("id") DO NOTHING`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"w1", "Alpha", "w2"}) {
			t.Fatalf("args = %#v, want [w1 Alpha w2]", args)
		}
	})
}
