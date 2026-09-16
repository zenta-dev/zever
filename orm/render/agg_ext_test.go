package render

import (
	"errors"
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

func plainGroup(col string) GroupTerm { return GroupTerm{Kind: GroupPlain, Column: col} }

func lowerNameFunc() *FuncExpr {
	return &FuncExpr{Name: "LOWER", Args: []Node{{Kind: KindColumn, Column: "name"}}}
}

func TestGroupedSelectAggregateFilter(t *testing.T) {
	t.Parallel()
	aggs := []Aggregate{{
		Func:   AggSum,
		Column: "amount",
		Alias:  "sum_amount",
		Filter: &Node{Kind: KindBinary, Column: "status", Op: OpEq, Value: "paid"},
	}}

	t.Run("postgres numbered placeholder", func(t *testing.T) {
		t.Parallel()
		q, args, err := GroupedSelect(
			postgres.New(), "orders",
			[]GroupTerm{plainGroup("category")}, aggs, Node{}, HavingNode{},
		)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category", SUM("amount") FILTER (WHERE "status" = $1) AS "sum_amount" FROM "orders" GROUP BY "category"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"paid"}) {
			t.Fatalf("args = %#v, want [paid]", args)
		}
	})

	t.Run("sqlite anonymous placeholder", func(t *testing.T) {
		q, args, err := GroupedSelect(
			sqlite.New(), "orders",
			[]GroupTerm{plainGroup("category")}, aggs, Node{}, HavingNode{},
		)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category", SUM("amount") FILTER (WHERE "status" = ?) AS "sum_amount" FROM "orders" GROUP BY "category"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"paid"}) {
			t.Fatalf("args = %#v, want [paid]", args)
		}
	})

	t.Run("base-only dialect gated", func(t *testing.T) {
		_, _, err := GroupedSelect(
			baseOnlyDialect{}, "orders",
			[]GroupTerm{plainGroup("category")}, aggs, Node{}, HavingNode{},
		)
		if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
			t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
		}
	})
}

func TestGroupedSelectCountDistinct(t *testing.T) {
	t.Parallel()
	aggs := []Aggregate{{Func: AggCount, Column: "name", Alias: "count_distinct_name", DistinctArg: true}}

	q, args, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, aggs, Node{}, HavingNode{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "category", COUNT(DISTINCT "name") AS "count_distinct_name" FROM "orders" GROUP BY "category"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}

func TestGroupedSelectDistinctWithoutColumnErrors(t *testing.T) {
	t.Parallel()
	aggs := []Aggregate{{Func: AggCount, Alias: "count", DistinctArg: true}}

	_, _, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, aggs, Node{}, HavingNode{})
	if err == nil {
		t.Fatalf("err = nil, want a DISTINCT-without-column error")
	}

	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want wrapping ErrUnsupported", err)
	}
}

func TestGroupedSelectExpressionGroup(t *testing.T) {
	t.Parallel()
	groups := []GroupTerm{{Kind: GroupPlain, Func: lowerNameFunc()}}

	q, _, err := GroupedSelect(sqlite.New(), "orders", groups, []Aggregate{{Func: AggCount, Alias: "count"}}, Node{}, HavingNode{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT LOWER("name"), COUNT(*) AS "count" FROM "orders" GROUP BY LOWER("name")`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

func TestGroupedSelectRollup(t *testing.T) {
	t.Parallel()
	groups := []GroupTerm{{Kind: GroupRollup, Terms: []GroupTerm{plainGroup("category"), plainGroup("region")}}}
	aggs := []Aggregate{{Func: AggCount, Alias: "count"}}

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		q, _, err := GroupedSelect(postgres.New(), "orders", groups, aggs, Node{}, HavingNode{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category", "region", COUNT(*) AS "count" FROM "orders" GROUP BY ROLLUP("category", "region")`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("sqlite gated", func(t *testing.T) {
		_, _, err := GroupedSelect(sqlite.New(), "orders", groups, aggs, Node{}, HavingNode{})
		if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
			t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
		}
	})
}

func TestGroupedSelectCube(t *testing.T) {
	t.Parallel()
	groups := []GroupTerm{{Kind: GroupCube, Terms: []GroupTerm{plainGroup("category"), plainGroup("region")}}}

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		q, _, err := GroupedSelect(postgres.New(), "orders", groups, nil, Node{}, HavingNode{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category", "region" FROM "orders" GROUP BY CUBE("category", "region")`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("sqlite gated", func(t *testing.T) {
		_, _, err := GroupedSelect(sqlite.New(), "orders", groups, nil, Node{}, HavingNode{})
		if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
			t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
		}
	})
}

func TestGroupedSelectGroupingSets(t *testing.T) {
	t.Parallel()
	groups := []GroupTerm{{Kind: GroupGroupingSets, Sets: [][]GroupTerm{
		{plainGroup("category"), plainGroup("region")},
		{plainGroup("category")},
		{},
	}}}

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()
		q, _, err := GroupedSelect(postgres.New(), "orders", groups, nil, Node{}, HavingNode{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category", "region" FROM "orders" GROUP BY GROUPING SETS (("category", "region"), ("category"), ())`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("sqlite gated", func(t *testing.T) {
		_, _, err := GroupedSelect(sqlite.New(), "orders", groups, nil, Node{}, HavingNode{})
		if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
			t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
		}
	})
}

func TestGroupedSelectHavingExpression(t *testing.T) {
	t.Parallel()
	having := HavingNode{
		Kind: HavingExpr,
		Expr: &Node{Kind: KindFunc, Func: lowerNameFunc(), Op: OpEq, Value: "apple"},
	}

	q, args, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "category" FROM "orders" GROUP BY "category" HAVING LOWER("name") = ?`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"apple"}) {
		t.Fatalf("args = %#v, want [apple]", args)
	}
}

func TestGroupedSelectSelectThenWhereThenGroupThenHavingArgOrder(t *testing.T) {
	t.Parallel()
	aggs := []Aggregate{{
		Func:   AggSum,
		Column: "amount",
		Alias:  "sum_amount",
		Filter: &Node{Kind: KindBinary, Column: "status", Op: OpEq, Value: "paid"},
	}}
	where := Node{Kind: KindBinary, Column: "region", Op: OpEq, Value: "west"}
	having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpGt, Value: 1}

	q, args, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, aggs, where, having)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "category", SUM("amount") FILTER (WHERE "status" = $1) AS "sum_amount" FROM "orders" WHERE "region" = $2 GROUP BY "category" HAVING COUNT(*) > $3`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"paid", "west", 1}) {
		t.Fatalf("args = %#v, want [paid west 1]", args)
	}
}

func TestGroupedSelectExpressionGroupArgDuplication(t *testing.T) {
	t.Parallel()
	fn := &FuncExpr{Name: "COALESCE", Args: []Node{{Kind: KindColumn, Column: "bio"}, {Kind: KindLit, Value: "n/a"}}}
	groups := []GroupTerm{{Kind: GroupPlain, Func: fn}}

	q, args, err := GroupedSelect(postgres.New(), "widgets", groups, nil, Node{}, HavingNode{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	// A scalar-expression GROUP BY is emitted in both the select list and the
	// GROUP BY clause, so its bound argument appears twice -- each occurrence
	// gets its own placeholder, in text order.
	want := `SELECT COALESCE("bio", $1) FROM "widgets" GROUP BY COALESCE("bio", $2)`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"n/a", "n/a"}) {
		t.Fatalf("args = %#v, want [n/a n/a]", args)
	}
}

func TestGroupedSelectFilteredAggregateInHaving(t *testing.T) {
	t.Parallel()
	filtered := Aggregate{Func: AggCount, Filter: &Node{Kind: KindBinary, Column: "status", Op: OpEq, Value: "paid"}}
	having := HavingNode{Kind: HavingLeaf, Agg: filtered, Op: OpGt, Value: 2}

	q, args, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "category" FROM "orders" GROUP BY "category" HAVING COUNT(*) FILTER (WHERE "status" = $1) > $2`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"paid", 2}) {
		t.Fatalf("args = %#v, want [paid 2]", args)
	}
}
