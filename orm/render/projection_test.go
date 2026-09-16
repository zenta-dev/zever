package render

import (
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestProjectedSelect pins the exact text a projected SELECT renders:
// aliased scalar expressions (function/CASE/column leaves) plus the ordinary
// WHERE/ORDER/LIMIT/OFFSET tail, with placeholder numbering flowing through
// the projection expressions first.
func TestProjectedSelect(t *testing.T) {
	t.Parallel()
	coalesce := Projection{
		Expr:  Node{Kind: KindFunc, Func: &FuncExpr{Name: "COALESCE", Args: []Node{{Kind: KindColumn, Column: "bio"}, {Kind: KindLit, Value: ""}}}},
		Alias: "bio",
	}
	lower := Projection{
		Expr:  Node{Kind: KindFunc, Func: &FuncExpr{Name: "LOWER", Args: []Node{{Kind: KindColumn, Column: "name"}}}},
		Alias: "lname",
	}
	plain := Projection{Expr: Node{Kind: KindColumn, Column: "id"}, Alias: "wid"}

	where := Node{Kind: KindBinary, Table: "widgets", Column: "quantity", Op: OpGt, Value: int64(15)}
	order := []OrderTerm{{Column: "id"}}

	got, args, err := ProjectedSelect(
		sqlite.New(), "widgets",
		[]Projection{coalesce, lower, plain},
		where, order, 5, 0, SelectModifiers{},
	)
	if err != nil {
		t.Fatalf("ProjectedSelect: %v", err)
	}

	want := `SELECT COALESCE("bio", ?) AS "bio", LOWER("name") AS "lname", "id" AS "wid" FROM "widgets" WHERE "quantity" > ? ORDER BY "id" ASC LIMIT ?`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}

	if !reflect.DeepEqual(args, []any{"", int64(15), 5}) {
		t.Fatalf("args = %#v, want [\"\" 15 5] (projection args first, in text order)", args)
	}
}

// TestProjectedSelectPostgresNumbering proves Postgres's $N placeholders are
// numbered sequentially across a projection-expression argument and the
// trailing WHERE.
func TestProjectedSelectPostgresNumbering(t *testing.T) {
	t.Parallel()
	coalesce := Projection{
		Expr:  Node{Kind: KindFunc, Func: &FuncExpr{Name: "COALESCE", Args: []Node{{Kind: KindColumn, Column: "bio"}, {Kind: KindLit, Value: "n/a"}}}},
		Alias: "bio",
	}

	where := Node{Kind: KindBinary, Table: "widgets", Column: "quantity", Op: OpGt, Value: int64(15)}

	got, args, err := ProjectedSelect(postgres.New(), "widgets", []Projection{coalesce}, where, nil, 0, 0, SelectModifiers{})
	if err != nil {
		t.Fatalf("ProjectedSelect: %v", err)
	}

	want := `SELECT COALESCE("bio", $1) AS "bio" FROM "widgets" WHERE "quantity" > $2`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}

	if !reflect.DeepEqual(args, []any{"n/a", int64(15)}) {
		t.Fatalf("args = %#v, want [n/a 15]", args)
	}
}

// TestProjectedSelectScalarSubquery proves a scalar subquery can be projected
// as an aliased output column, rendered inline as `(SELECT ...) AS "alias"`.
func TestProjectedSelectScalarSubquery(t *testing.T) {
	t.Parallel()
	sub := Projection{
		Expr:  Node{Kind: KindSubquery, Value: Subquery{Table: "orders", Columns: []string{"amount"}}},
		Alias: "amt",
	}

	got, args, err := ProjectedSelect(sqlite.New(), "widgets", []Projection{sub}, Node{}, nil, 0, 0, SelectModifiers{})
	if err != nil {
		t.Fatalf("ProjectedSelect: %v", err)
	}

	want := `SELECT (SELECT "amount" FROM "orders") AS "amt" FROM "widgets"`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

// TestProjectedSelectDistinct proves the DISTINCT modifier composes with an
// expression projection.
func TestProjectedSelectDistinct(t *testing.T) {
	t.Parallel()
	p := Projection{
		Expr:  Node{Kind: KindFunc, Func: &FuncExpr{Name: "LOWER", Args: []Node{{Kind: KindColumn, Column: "name"}}}},
		Alias: "lname",
	}

	got, _, err := ProjectedSelect(sqlite.New(), "widgets", []Projection{p}, Node{}, nil, 0, 0, SelectModifiers{Distinct: true})
	if err != nil {
		t.Fatalf("ProjectedSelect: %v", err)
	}

	want := `SELECT DISTINCT LOWER("name") AS "lname" FROM "widgets"`
	if got != want {
		t.Fatalf("query = %q, want %q", got, want)
	}
}

// TestProjectedSelectEmptyErrors proves a projection list with no entries is
// a rendering-time error rather than an invalid `SELECT  FROM ...` statement.
func TestProjectedSelectEmptyErrors(t *testing.T) {
	t.Parallel()
	if _, _, err := ProjectedSelect(sqlite.New(), "widgets", nil, Node{}, nil, 0, 0, SelectModifiers{}); err == nil {
		t.Fatalf("ProjectedSelect(nil projections) err = nil, want an error")
	}
}

func TestProjectedSelectErrorPaths(t *testing.T) {
	t.Parallel()

	good := Projection{Expr: Node{Kind: KindColumn, Column: "id"}, Alias: "wid"}

	t.Run("expression error propagates", func(t *testing.T) {
		t.Parallel()

		bad := Projection{Expr: Node{Kind: KindBinary, Column: "id", Op: OpEq, Value: 1}, Alias: "x"}

		_, _, err := ProjectedSelect(sqlite.New(), "widgets", []Projection{bad}, Node{}, nil, 0, 0, SelectModifiers{})
		if err == nil {
			t.Fatal("err = nil, want the projection expression error")
		}
	})

	t.Run("modifier error propagates", func(t *testing.T) {
		t.Parallel()

		mods := SelectModifiers{Distinct: true, Lock: LockForUpdate}

		_, _, err := ProjectedSelect(sqlite.New(), "widgets", []Projection{good}, Node{}, nil, 0, 0, mods)
		if err == nil {
			t.Fatal("err = nil, want the DISTINCT+lock error")
		}
	})

	t.Run("tail error propagates", func(t *testing.T) {
		t.Parallel()

		badWhere := Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: 1}

		_, _, err := ProjectedSelect(sqlite.New(), "widgets", []Projection{good}, badWhere, nil, 0, 0, SelectModifiers{})
		if err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})
}
