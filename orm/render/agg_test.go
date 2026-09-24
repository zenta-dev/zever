package render

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

func TestGroupedSelect(t *testing.T) {
	t.Parallel()
	t.Run("group by and aggregates, no where/having", func(t *testing.T) {
		t.Parallel()
		q, args, err := GroupedSelect(
			sqlite.New(),
			"orders",
			[]GroupTerm{plainGroup("category")},
			[]Aggregate{{Func: AggCount, Alias: "count"}, {Func: AggSum, Column: "amount", Alias: "sum_amount"}},
			Node{},
			HavingNode{},
		)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category", COUNT(*) AS "count", SUM("amount") AS "sum_amount" FROM "orders" GROUP BY "category"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if len(args) != 0 {
			t.Fatalf("args = %#v, want none", args)
		}
	})

	t.Run("no group columns falls back to COUNT(*)", func(t *testing.T) {
		q, _, err := GroupedSelect(sqlite.New(), "orders", nil, nil, Node{}, HavingNode{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT COUNT(*) FROM "orders"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("having renders after group by", func(t *testing.T) {
		having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpGt, Value: 2}

		q, args, err := GroupedSelect(
			sqlite.New(),
			"orders",
			[]GroupTerm{plainGroup("category")},
			[]Aggregate{{Func: AggCount, Alias: "count"}},
			Node{},
			having,
		)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category", COUNT(*) AS "count" FROM "orders" GROUP BY "category" HAVING COUNT(*) > ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{2}) {
			t.Fatalf("args = %#v, want [2]", args)
		}
	})

	t.Run("where and having arg ordering", func(t *testing.T) {
		where := Node{Kind: KindBinary, Column: "region", Op: OpEq, Value: "west"}
		having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggSum, Column: "amount"}, Op: OpGte, Value: 100}

		q, args, err := stubPostgresGroupedSelect(where, having)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category", SUM("amount") AS "sum_amount" FROM "orders" WHERE "region" = $1 GROUP BY "category" HAVING SUM("amount") >= $2`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		// WHERE's arg ("west") must come before HAVING's arg (100), in the
		// same order their placeholders ($1 then $2) appear in the SQL text.
		if !reflect.DeepEqual(args, []any{"west", 100}) {
			t.Fatalf("args = %#v, want [\"west\" 100]", args)
		}
	})

	t.Run("having AND compound", func(t *testing.T) {
		having := HavingNode{
			Kind:     HavingCompound,
			Compound: CompoundAnd,
			Children: []HavingNode{
				{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpGt, Value: 1},
				{Kind: HavingLeaf, Agg: Aggregate{Func: AggSum, Column: "amount"}, Op: OpLt, Value: 1000},
			},
		}

		q, args, err := GroupedSelect(
			sqlite.New(),
			"orders",
			[]GroupTerm{plainGroup("category")},
			[]Aggregate{{Func: AggCount, Alias: "count"}, {Func: AggSum, Column: "amount", Alias: "sum_amount"}},
			Node{},
			having,
		)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category", COUNT(*) AS "count", SUM("amount") AS "sum_amount" FROM "orders" GROUP BY "category" HAVING (COUNT(*) > ? AND SUM("amount") < ?)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{1, 1000}) {
			t.Fatalf("args = %#v, want [1 1000]", args)
		}
	})

	t.Run("having value type mismatch is a rendering error, not a panic", func(t *testing.T) {
		having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggSum, Column: "amount"}, Op: OpGt, Value: "not-a-number"}

		_, _, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
		if err == nil {
			t.Fatalf("err = nil, want a type-mismatch error")
		}

		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err = %v, want wrapping ErrUnsupported", err)
		}
	})

	t.Run("having value type mismatch is tolerated for MIN/MAX", func(t *testing.T) {
		having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggMax, Column: "name"}, Op: OpGt, Value: "M"}

		q, args, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
		if err != nil {
			t.Fatalf("err = %v, want nil (MIN/MAX allow non-numeric HAVING values)", err)
		}

		want := `SELECT "category" FROM "orders" GROUP BY "category" HAVING MAX("name") > ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"M"}) {
			t.Fatalf("args = %#v, want [\"M\"]", args)
		}
	})
}

func stubPostgresGroupedSelect(where Node, having HavingNode) (string, []any, error) {
	return GroupedSelect(
		fakePostgres{},
		"orders",
		[]GroupTerm{plainGroup("category")},
		[]Aggregate{{Func: AggSum, Column: "amount", Alias: "sum_amount"}},
		where,
		having,
	)
}

func TestGroupedSelectScalarAggFuncNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		agg  Aggregate
		want string
	}{
		{"avg", Aggregate{Func: AggAvg, Column: "amount", Alias: "a"}, `SELECT "category", AVG("amount") AS "a" FROM "orders" GROUP BY "category"`},
		{"min", Aggregate{Func: AggMin, Column: "amount", Alias: "a"}, `SELECT "category", MIN("amount") AS "a" FROM "orders" GROUP BY "category"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			q, _, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, []Aggregate{tc.agg}, Node{}, HavingNode{})
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if q != tc.want {
				t.Fatalf("query = %q, want %q", q, tc.want)
			}
		})
	}

	if got := AggFunc(99).name(); got != "COUNT" {
		t.Fatalf("AggFunc(99).name() = %q, want COUNT", got)
	}

	for _, tc := range []struct {
		fn   AggFunc
		want string
	}{
		{AggArrayAgg, "array_agg"},
		{AggStringAgg, "string_agg"},
		{AggGroupConcat, "group_concat"},
	} {
		if got := tc.fn.name(); got != tc.want {
			t.Fatalf("name() = %q, want %q", got, tc.want)
		}
	}
}

func TestGroupedSelectHavingNonNumericAggSkipsTypeCheck(t *testing.T) {
	t.Parallel()

	having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggGroupConcat, Column: "name"}, Op: OpGt, Value: "M"}

	q, _, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "category" FROM "orders" GROUP BY "category" HAVING group_concat("name") > ?`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

func TestGroupedSelectHavingBoolValueRejected(t *testing.T) {
	t.Parallel()

	having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggSum, Column: "amount"}, Op: OpGt, Value: true}

	_, _, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("err = %v, want wrapping ErrUnsupported", err)
	}
}

func TestGroupedSelectFilterErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("filter render error propagates", func(t *testing.T) {
		t.Parallel()

		aggs := []Aggregate{{
			Func: AggSum, Column: "amount", Alias: "s",
			Filter: &Node{Kind: KindBinary, Column: "status", Op: OpEqAny, Value: "paid"},
		}}

		_, _, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, aggs, Node{}, HavingNode{})
		if err == nil {
			t.Fatal("err = nil, want the FILTER predicate error")
		}
	})

	t.Run("empty filter predicate rejected", func(t *testing.T) {
		t.Parallel()

		aggs := []Aggregate{{Func: AggSum, Column: "amount", Alias: "s", Filter: &Node{}}}

		_, _, err := GroupedSelect(sqlite.New(), "orders", []GroupTerm{plainGroup("category")}, aggs, Node{}, HavingNode{})
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err = %v, want wrapping ErrUnsupported", err)
		}
	})
}

func TestGroupedSelectOrderedAggErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("ordered aggregate without column", func(t *testing.T) {
		t.Parallel()

		_, _, err := GroupedSelect(postgres.New(), "orders", nil,
			[]Aggregate{{Func: AggArrayAgg, Alias: "a"}}, Node{}, HavingNode{})
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err = %v, want wrapping ErrUnsupported", err)
		}
	})

	t.Run("ordered aggregate family gated per dialect", func(t *testing.T) {
		t.Parallel()

		_, _, err := GroupedSelect(sqlite.New(), "orders", nil,
			[]Aggregate{{Func: AggArrayAgg, Column: "tags", Alias: "a"}}, Node{}, HavingNode{})
		if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
			t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
		}
	})

	t.Run("non-ordered func rejected by orderedAggFuncName", func(t *testing.T) {
		t.Parallel()

		if _, err := orderedAggFuncName(postgres.New(), AggCount); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err = %v, want wrapping ErrUnsupported", err)
		}
	})

	t.Run("distinct ordered aggregate", func(t *testing.T) {
		t.Parallel()

		q, _, err := GroupedSelect(sqlite.New(), "orders", nil,
			[]Aggregate{{Func: AggGroupConcat, Column: "name", Alias: "a", DistinctArg: true}}, Node{}, HavingNode{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT group_concat(DISTINCT "name") AS "a" FROM "orders"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("multi-term in-aggregate order by", func(t *testing.T) {
		t.Parallel()

		q, _, err := GroupedSelect(sqlite.New(), "orders", nil,
			[]Aggregate{{Func: AggGroupConcat, Column: "name", Alias: "a",
				Order: []OrderRef{{Column: "qty"}, {Column: "name", Desc: true}}}}, Node{}, HavingNode{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT group_concat("name" ORDER BY "qty" ASC, "name" DESC) AS "a" FROM "orders"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("empty in-aggregate order column rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := GroupedSelect(sqlite.New(), "orders", nil,
			[]Aggregate{{Func: AggGroupConcat, Column: "name", Alias: "a", Order: []OrderRef{{}}}}, Node{}, HavingNode{})
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err = %v, want wrapping ErrUnsupported", err)
		}
	})
}

func TestGroupedSelectGroupErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		groups []GroupTerm
		want   string
	}{
		{"leaf without column or expression", []GroupTerm{{Kind: GroupPlain}}, "neither a column nor an expression"},
		{"empty rollup", []GroupTerm{{Kind: GroupRollup}}, "requires at least one grouping term"},
		{"nested rollup", []GroupTerm{{Kind: GroupRollup, Terms: []GroupTerm{{Kind: GroupCube, Terms: []GroupTerm{plainGroup("a")}}}}}, "nested grouping constructs"},
		{"rollup empty leaf", []GroupTerm{{Kind: GroupRollup, Terms: []GroupTerm{{Kind: GroupPlain}}}}, "neither a column nor an expression"},
		{"rollup failing expression leaf", []GroupTerm{{Kind: GroupRollup, Terms: []GroupTerm{{Kind: GroupPlain, Func: &FuncExpr{}}}}}, "empty function name"},
		{"empty grouping sets", []GroupTerm{{Kind: GroupGroupingSets}}, "at least one set"},
		{"nested grouping sets", []GroupTerm{{Kind: GroupGroupingSets, Sets: [][]GroupTerm{{{Kind: GroupCube}}}}}, "nested grouping constructs"},
		{"grouping sets empty leaf", []GroupTerm{{Kind: GroupGroupingSets, Sets: [][]GroupTerm{{{Kind: GroupPlain}}}}}, "neither a column nor an expression"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := GroupedSelect(postgres.New(), "orders", tc.groups, nil, Node{}, HavingNode{})
			if err == nil {
				t.Fatal("err = nil, want a grouping error")
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}

	t.Run("unknown group kind rejected", func(t *testing.T) {
		t.Parallel()

		// An out-of-range kind never survives flattenGroupTerms (it falls
		// into the leaf default there), so it is exercised directly against
		// renderGroupTerm.
		_, _, err := renderGroupTerm(postgres.New(), GroupTerm{Kind: GroupKind(99)}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want an unknown-kind error")
		}

		if !strings.Contains(err.Error(), "unknown group term kind") {
			t.Fatalf("err = %v, want it to mention unknown group term kind", err)
		}
	})

	t.Run("keyword group leaf error propagates", func(t *testing.T) {
		t.Parallel()

		// A failing leaf inside ROLLUP can only be exercised directly: the
		// grouped renderers validate the same leaves in their select lists
		// first, so a public call fails there before GROUP BY rendering.
		_, _, err := renderKeywordGroup(postgres.New(), "ROLLUP",
			[]GroupTerm{{Kind: GroupPlain}}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want the leaf error")
		}
	})

	t.Run("grouping sets leaf error propagates", func(t *testing.T) {
		t.Parallel()

		_, _, err := renderGroupingSets(postgres.New(),
			[][]GroupTerm{{{Kind: GroupPlain}}}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want the leaf error")
		}
	})

	t.Run("where error propagates", func(t *testing.T) {
		t.Parallel()

		where := Node{Kind: KindBinary, Column: "region", Op: OpEqAny, Value: "west"}

		_, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, where, HavingNode{})
		if err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})
}

func TestGroupedSelectHavingShapes(t *testing.T) {
	t.Parallel()

	t.Run("nil having expression rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, HavingNode{Kind: HavingExpr})
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err = %v, want wrapping ErrUnsupported", err)
		}
	})

	t.Run("unknown having kind renders no having", func(t *testing.T) {
		t.Parallel()

		q, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, HavingNode{Kind: HavingKind(99)})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if strings.Contains(q, "HAVING") {
			t.Fatalf("query = %q, want no HAVING clause", q)
		}
	})

	t.Run("having leaf bad operator", func(t *testing.T) {
		t.Parallel()

		having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpEqAny, Value: 1}

		_, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
		if err == nil {
			t.Fatal("err = nil, want an operator error")
		}
	})

	t.Run("having leaf aggregate error propagates", func(t *testing.T) {
		t.Parallel()

		having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount, DistinctArg: true}, Op: OpGt, Value: 1}

		_, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("err = %v, want wrapping ErrUnsupported", err)
		}
	})

	t.Run("having not without child renders no having", func(t *testing.T) {
		t.Parallel()

		q, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{},
			HavingNode{Kind: HavingCompound, Compound: CompoundNot})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if strings.Contains(q, "HAVING") {
			t.Fatalf("query = %q, want no HAVING clause", q)
		}
	})

	t.Run("having not wraps child", func(t *testing.T) {
		t.Parallel()

		having := HavingNode{Kind: HavingCompound, Compound: CompoundNot, Children: []HavingNode{
			{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpGt, Value: 1},
		}}

		q, args, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category" FROM "orders" GROUP BY "category" HAVING NOT (COUNT(*) > $1)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{1}) {
			t.Fatalf("args = %#v, want [1]", args)
		}
	})

	t.Run("having not child error propagates", func(t *testing.T) {
		t.Parallel()

		having := HavingNode{Kind: HavingCompound, Compound: CompoundNot, Children: []HavingNode{
			{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpEqAny, Value: 1},
		}}

		_, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
		if err == nil {
			t.Fatal("err = nil, want the child error")
		}
	})

	t.Run("having or joins with OR", func(t *testing.T) {
		t.Parallel()

		having := HavingNode{Kind: HavingCompound, Compound: CompoundOr, Children: []HavingNode{
			{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpGt, Value: 1},
			{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpLt, Value: 10},
		}}

		q, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category" FROM "orders" GROUP BY "category" HAVING (COUNT(*) > $1 OR COUNT(*) < $2)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("having and skips empty children", func(t *testing.T) {
		t.Parallel()

		having := HavingNode{Kind: HavingCompound, Compound: CompoundAnd, Children: []HavingNode{
			{Kind: HavingNone},
			{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpGt, Value: 1},
		}}

		q, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "category" FROM "orders" GROUP BY "category" HAVING (COUNT(*) > $1)`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("having and child error propagates", func(t *testing.T) {
		t.Parallel()

		having := HavingNode{Kind: HavingCompound, Compound: CompoundAnd, Children: []HavingNode{
			{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpGt, Value: 1},
			{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpEqAny, Value: 1},
		}}

		_, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
		if err == nil {
			t.Fatal("err = nil, want the child error")
		}
	})
}

func TestGroupedSelectHavingNonComparisonOps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		op   Op
		want string
	}{
		{"like", OpLike, `SELECT "category" FROM "orders" GROUP BY "category" HAVING COUNT(*) LIKE $1`},
		{"between", OpBetween, `SELECT "category" FROM "orders" GROUP BY "category" HAVING COUNT(*) BETWEEN $1`},
		{"in", OpIn, `SELECT "category" FROM "orders" GROUP BY "category" HAVING COUNT(*) = $1`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			having := HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: tc.op, Value: 1}

			q, _, err := GroupedSelect(postgres.New(), "orders", []GroupTerm{plainGroup("category")}, nil, Node{}, having)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if q != tc.want {
				t.Fatalf("query = %q, want %q", q, tc.want)
			}
		})
	}
}
