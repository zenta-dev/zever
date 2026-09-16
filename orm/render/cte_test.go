package render

import (
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

func TestSetOp(t *testing.T) {
	t.Parallel()
	t.Run("union with where args on both sides", func(t *testing.T) {
		t.Parallel()
		leftWhere := Node{Kind: KindBinary, Op: OpEq, Column: "name", Value: "alpha"}
		rightWhere := Node{Kind: KindBinary, Op: OpEq, Column: "name", Value: "omega"}

		q, args, err := SetOp(
			sqlite.New(),
			SetOpUnion,
			"widgets", []string{"id", "name"}, leftWhere,
			"widgets", []string{"id", "name"}, rightWhere,
			nil, 0, 0,
		)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id", "name" FROM "widgets" WHERE "name" = ? UNION SELECT "id", "name" FROM "widgets" WHERE "name" = ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"alpha", "omega"}) {
			t.Fatalf("args = %#v, want [alpha omega]", args)
		}
	})

	t.Run("postgres numbered placeholders stay sequential", func(t *testing.T) {
		leftWhere := Node{Kind: KindBinary, Op: OpEq, Column: "name", Value: "alpha"}

		q, args, err := SetOp(
			postgres.New(),
			SetOpExcept,
			"widgets", []string{"id"}, leftWhere,
			"widgets", []string{"id"}, Node{},
			[]OrderTerm{{Column: "id", Desc: true}}, 5, 10,
		)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id" FROM "widgets" WHERE "name" = $1 EXCEPT SELECT "id" FROM "widgets" ORDER BY "id" DESC LIMIT $2 OFFSET $3`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"alpha", 5, 10}) {
			t.Fatalf("args = %#v, want [alpha 5 10]", args)
		}
	})

	t.Run("intersect keyword", func(t *testing.T) {
		q, _, err := SetOp(sqlite.New(), SetOpIntersect, "a", []string{"id"}, Node{}, "b", []string{"id"}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `SELECT "id" FROM "a" INTERSECT SELECT "id" FROM "b"`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("union all keyword", func(t *testing.T) {
		q, _, err := SetOp(sqlite.New(), SetOpUnionAll, "a", []string{"id"}, Node{}, "b", []string{"id"}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `SELECT "id" FROM "a" UNION ALL SELECT "id" FROM "b"`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

func TestSelectWith(t *testing.T) {
	t.Parallel()
	t.Run("plain body", func(t *testing.T) {
		t.Parallel()
		body := CTEBody{
			Kind:    CTEBodyPlain,
			Table:   "widgets",
			Columns: []string{"id", "name"},
			Where:   Node{Kind: KindBinary, Op: OpEq, Column: "name", Value: "alpha"},
		}

		outerWhere := Node{Kind: KindBinary, Op: OpGt, Column: "quantity", Value: 5}

		q, args, err := SelectWith(sqlite.New(), "ranked", false, body, []string{"id", "name"}, outerWhere, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `WITH "ranked" AS (SELECT "id", "name" FROM "widgets" WHERE "name" = ?) SELECT "id", "name" FROM "ranked" WHERE "quantity" > ?`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"alpha", 5}) {
			t.Fatalf("args = %#v, want [alpha 5]", args)
		}
	})

	t.Run("postgres shared counter across body and outer", func(t *testing.T) {
		body := CTEBody{
			Kind:    CTEBodyPlain,
			Table:   "widgets",
			Columns: []string{"id", "name"},
			Where:   Node{Kind: KindBinary, Op: OpEq, Column: "name", Value: "alpha"},
		}

		outerWhere := Node{Kind: KindBinary, Op: OpEq, Column: "name", Value: "omega"}

		q, args, err := SelectWith(postgres.New(), "ranked", false, body, []string{"id", "name"}, outerWhere, nil, 10, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `WITH "ranked" AS (SELECT "id", "name" FROM "widgets" WHERE "name" = $1) SELECT "id", "name" FROM "ranked" WHERE "name" = $2 LIMIT $3`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"alpha", "omega", 10}) {
			t.Fatalf("args = %#v, want [alpha omega 10]", args)
		}
	})

	t.Run("recursive set-op body", func(t *testing.T) {
		base := CTEBody{
			Kind:    CTEBodyPlain,
			Table:   "org_units",
			Columns: []string{"id", "name", "parent_id"},
			Where:   Node{Kind: KindBinary, Op: OpIsNull, Column: "parent_id"},
		}

		recursive := CTEBody{
			Kind:      CTEBodyPlain,
			Table:     "org_units",
			Columns:   []string{"id", "name", "parent_id"},
			JoinTable: "org_tree",
			ParentCol: "parent_id",
			ChildCol:  "id",
		}

		body := CTEBody{
			Kind:      CTEBodySetOp,
			SetOp:     SetOpUnionAll,
			LeftBody:  &base,
			RightBody: &recursive,
		}

		q, _, err := SelectWith(sqlite.New(), "org_tree", true, body, []string{"id", "name", "parent_id"}, Node{}, []OrderTerm{{Column: "name"}}, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `WITH RECURSIVE "org_tree" AS (SELECT "id", "name", "parent_id" FROM "org_units" WHERE "parent_id" IS NULL UNION ALL SELECT "org_units"."id", "org_units"."name", "org_units"."parent_id" FROM "org_units" JOIN "org_tree" ON "org_units"."parent_id" = "org_tree"."id") SELECT "id", "name", "parent_id" FROM "org_tree" ORDER BY "name" ASC`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("set-op operand order/limit are dropped", func(t *testing.T) {
		left := CTEBody{
			Kind:    CTEBodyPlain,
			Table:   "widgets",
			Columns: []string{"id"},
			Limit:   5,
		}

		body := CTEBody{
			Kind:     CTEBodySetOp,
			SetOp:    SetOpUnion,
			LeftBody: &left,
			RightBody: &CTEBody{
				Kind:    CTEBodyPlain,
				Table:   "widgets",
				Columns: []string{"id"},
			},
		}

		q, _, err := SelectWith(sqlite.New(), "w", false, body, []string{"id"}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `WITH "w" AS (SELECT "id" FROM "widgets" UNION SELECT "id" FROM "widgets") SELECT "id" FROM "w"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("join body projects both sides", func(t *testing.T) {
		body := CTEBody{
			Kind:         CTEBodyJoin,
			Table:        "users",
			Columns:      []string{"id", "email"},
			RightTable:   "orders",
			RightColumns: []string{"id", "amount_cents"},
			JoinType:     InnerJoin,
			ParentCol:    "id",
			ChildCol:     "user_id",
			Where:        Node{Kind: KindBinary, Op: OpEq, Table: "users", Column: "email", Value: "a@b.c"},
		}

		q, args, err := SelectWith(sqlite.New(), "u_orders", false, body, []string{"id", "email", "id", "amount_cents"}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `WITH "u_orders" AS (SELECT "users"."id", "users"."email", "orders"."id", "orders"."amount_cents" FROM "users" INNER JOIN "orders" ON "users"."id" = "orders"."user_id" WHERE "users"."email" = ?) SELECT "id", "email", "id", "amount_cents" FROM "u_orders"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"a@b.c"}) {
			t.Fatalf("args = %#v, want [a@b.c]", args)
		}
	})

	t.Run("grouped body", func(t *testing.T) {
		body := CTEBody{
			Kind:      CTEBodyGrouped,
			Table:     "orders",
			GroupCols: []string{"category"},
			Aggs:      []Aggregate{{Func: AggCount, Alias: "count"}, {Func: AggSum, Column: "amount_cents", Alias: "sum_amount_cents"}},
		}

		q, _, err := SelectWith(sqlite.New(), "cats", false, body, []string{"category", "count", "sum_amount_cents"}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `WITH "cats" AS (SELECT "category", COUNT(*) AS "count", SUM("amount_cents") AS "sum_amount_cents" FROM "orders" GROUP BY "category") SELECT "category", "count", "sum_amount_cents" FROM "cats"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})
}

// TestSelectWithMaterialization pins the rendered `[NOT] MATERIALIZED` CTE
// modifier, including that the zero-value CTEMaterializeDefault keeps the
// pre-existing `AS (` spelling byte-for-byte.
func TestSelectWithMaterialization(t *testing.T) {
	t.Parallel()
	base := CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id"}}

	cases := []struct {
		name string
		m    CTEMaterialization
		want string
	}{
		{
			"default emits no hint",
			CTEMaterializeDefault,
			`WITH "w" AS (SELECT "id" FROM "widgets") SELECT "id" FROM "w"`,
		},
		{
			"materialized",
			CTEMaterializeAlways,
			`WITH "w" AS MATERIALIZED (SELECT "id" FROM "widgets") SELECT "id" FROM "w"`,
		},
		{
			"not materialized",
			CTEMaterializeNever,
			`WITH "w" AS NOT MATERIALIZED (SELECT "id" FROM "widgets") SELECT "id" FROM "w"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := base
			body.Materialization = tc.m

			q, _, err := SelectWith(sqlite.New(), "w", false, body, []string{"id"}, Node{}, nil, 0, 0)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if q != tc.want {
				t.Fatalf("query = %q, want %q", q, tc.want)
			}
		})
	}
}

// TestCountWithMaterialization pins the modifier on the COUNT analog too.
func TestCountWithMaterialization(t *testing.T) {
	t.Parallel()
	body := CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id"}, Materialization: CTEMaterializeAlways}

	q, _, err := CountWith(sqlite.New(), "w", false, body)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `WITH "w" AS MATERIALIZED (SELECT "id" FROM "widgets") SELECT COUNT(*) FROM "w"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

// TestSelectWithSearchCycle pins the recursive `SEARCH DEPTH FIRST BY ... SET`
// and `CYCLE ... SET ... USING` clauses: both render after the recursive
// body's closing paren and before the outer SELECT.
func TestSelectWithSearchCycle(t *testing.T) {
	t.Parallel()
	base := CTEBody{
		Kind:    CTEBodyPlain,
		Table:   "org_units",
		Columns: []string{"id"},
		Where:   Node{Kind: KindBinary, Op: OpIsNull, Column: "parent_id"},
	}

	recursive := CTEBody{
		Kind:      CTEBodyPlain,
		Table:     "org_units",
		Columns:   []string{"id"},
		JoinTable: "org_tree",
		ParentCol: "parent_id",
		ChildCol:  "id",
	}

	body := CTEBody{
		Kind:       CTEBodySetOp,
		SetOp:      SetOpUnionAll,
		LeftBody:   &base,
		RightBody:  &recursive,
		SearchBy:   []string{"id"},
		SearchSet:  "ordercol",
		CycleBy:    []string{"id"},
		CycleSet:   "is_cycle",
		CycleUsing: "path",
	}

	q, _, err := SelectWith(postgres.New(), "org_tree", true, body, []string{"id"}, Node{}, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `WITH RECURSIVE "org_tree" AS (SELECT "id" FROM "org_units" WHERE "parent_id" IS NULL UNION ALL SELECT "org_units"."id" FROM "org_units" JOIN "org_tree" ON "org_units"."parent_id" = "org_tree"."id") SEARCH DEPTH FIRST BY "id" SET "ordercol" CYCLE "id" SET "is_cycle" USING "path" SELECT "id" FROM "org_tree"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

func TestCountWith(t *testing.T) {
	t.Parallel()
	t.Run("plain body", func(t *testing.T) {
		t.Parallel()
		body := CTEBody{
			Kind:    CTEBodyPlain,
			Table:   "widgets",
			Columns: []string{"id"},
			Where:   Node{Kind: KindBinary, Op: OpEq, Column: "name", Value: "alpha"},
		}

		q, args, err := CountWith(sqlite.New(), "w", false, body)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `WITH "w" AS (SELECT "id" FROM "widgets" WHERE "name" = ?) SELECT COUNT(*) FROM "w"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"alpha"}) {
			t.Fatalf("args = %#v, want [alpha]", args)
		}
	})
}

func TestSetOpErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("unknown op keyword defaults to UNION", func(t *testing.T) {
		t.Parallel()

		if got := SetOpOp(99).keyword(); got != "UNION" {
			t.Fatalf("keyword() = %q, want UNION", got)
		}
	})

	t.Run("left where error propagates", func(t *testing.T) {
		t.Parallel()

		bad := Node{Kind: KindBinary, Column: "name", Op: OpEqAny, Value: "alpha"}

		_, _, err := SetOp(sqlite.New(), SetOpUnion,
			"widgets", []string{"id"}, bad,
			"widgets", []string{"id"}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the left WHERE error")
		}
	})

	t.Run("right where error propagates", func(t *testing.T) {
		t.Parallel()

		bad := Node{Kind: KindBinary, Column: "name", Op: OpEqAny, Value: "omega"}

		_, _, err := SetOp(sqlite.New(), SetOpUnion,
			"widgets", []string{"id"}, Node{},
			"widgets", []string{"id"}, bad, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the right WHERE error")
		}
	})

	t.Run("order error propagates", func(t *testing.T) {
		t.Parallel()

		badOrder := []OrderTerm{{Func: &FuncExpr{}}}

		_, _, err := SetOp(sqlite.New(), SetOpUnion,
			"widgets", []string{"id"}, Node{},
			"widgets", []string{"id"}, Node{}, badOrder, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the ORDER BY error")
		}
	})
}

func TestSelectWithErrorPaths(t *testing.T) {
	t.Parallel()

	plain := func(where Node) CTEBody {
		return CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id"}, Where: where}
	}

	t.Run("body error propagates", func(t *testing.T) {
		t.Parallel()

		bad := plain(Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: "w1"})

		_, _, err := SelectWith(sqlite.New(), "w", false, bad, []string{"id"}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the body error")
		}
	})

	t.Run("outer where error propagates", func(t *testing.T) {
		t.Parallel()

		bad := Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: "w1"}

		_, _, err := SelectWith(sqlite.New(), "w", false, plain(Node{}), []string{"id"}, bad, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the outer WHERE error")
		}
	})

	t.Run("outer order error propagates", func(t *testing.T) {
		t.Parallel()

		badOrder := []OrderTerm{{Func: &FuncExpr{}}}

		_, _, err := SelectWith(sqlite.New(), "w", false, plain(Node{}), []string{"id"}, Node{}, badOrder, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the outer ORDER BY error")
		}
	})

	t.Run("nil set-op sides render empty body", func(t *testing.T) {
		t.Parallel()

		q, _, err := SelectWith(sqlite.New(), "w", false,
			CTEBody{Kind: CTEBodySetOp, SetOp: SetOpUnion}, []string{"id"}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `WITH "w" AS () SELECT "id" FROM "w"`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("unknown body kind renders empty body", func(t *testing.T) {
		t.Parallel()

		q, _, err := SelectWith(sqlite.New(), "w", false,
			CTEBody{Kind: CTEBodyKind(99)}, []string{"id"}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `WITH "w" AS () SELECT "id" FROM "w"`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("set-op left error propagates", func(t *testing.T) {
		t.Parallel()

		left := CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id"},
			Where: Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: 1}}
		right := CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id"}}

		_, _, err := SelectWith(sqlite.New(), "w", false,
			CTEBody{Kind: CTEBodySetOp, SetOp: SetOpUnion, LeftBody: &left, RightBody: &right},
			[]string{"id"}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the left body error")
		}
	})

	t.Run("set-op right error propagates", func(t *testing.T) {
		t.Parallel()

		left := CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id"}}
		right := CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id"},
			Where: Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: 1}}

		_, _, err := SelectWith(sqlite.New(), "w", false,
			CTEBody{Kind: CTEBodySetOp, SetOp: SetOpUnion, LeftBody: &left, RightBody: &right},
			[]string{"id"}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the right body error")
		}
	})
}

func TestCountWithRecursive(t *testing.T) {
	t.Parallel()

	body := CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id"}}

	q, _, err := CountWith(sqlite.New(), "w", true, body)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	if want := `WITH RECURSIVE "w" AS (SELECT "id" FROM "widgets") SELECT COUNT(*) FROM "w"`; q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

func TestCountWithBodyError(t *testing.T) {
	t.Parallel()

	bad := CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id"},
		Where: Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: 1}}

	_, _, err := CountWith(sqlite.New(), "w", false, bad)
	if err == nil {
		t.Fatal("err = nil, want the body error")
	}
}

func TestSelectWithJoinBodyTail(t *testing.T) {
	t.Parallel()

	body := CTEBody{
		Kind:         CTEBodyJoin,
		Table:        "users",
		Columns:      []string{"id"},
		RightTable:   "orders",
		RightColumns: []string{"id"},
		JoinType:     InnerJoin,
		ParentCol:    "id",
		ChildCol:     "user_id",
		Order:        []OrderTerm{{Column: "email"}},
		OrderRight:   []OrderTerm{{Column: "amount_cents", Desc: true}},
		Limit:        5,
		Offset:       2,
	}

	q, args, err := SelectWith(postgres.New(), "u_orders", false, body, []string{"id", "id"}, Node{}, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `WITH "u_orders" AS (SELECT "users"."id", "orders"."id" FROM "users" INNER JOIN "orders" ON "users"."id" = "orders"."user_id" ORDER BY "users"."email" ASC, "orders"."amount_cents" DESC LIMIT $1 OFFSET $2) SELECT "id", "id" FROM "u_orders"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{5, 2}) {
		t.Fatalf("args = %#v, want [5 2]", args)
	}
}

func TestSelectWithJoinBodyWhereError(t *testing.T) {
	t.Parallel()

	body := CTEBody{
		Kind:         CTEBodyJoin,
		Table:        "users",
		Columns:      []string{"id"},
		RightTable:   "orders",
		RightColumns: []string{"id"},
		JoinType:     InnerJoin,
		ParentCol:    "id",
		ChildCol:     "user_id",
		Where:        Node{Kind: KindBinary, Column: "email", Op: OpEqAny, Value: "a@b.c"},
	}

	_, _, err := SelectWith(sqlite.New(), "u_orders", false, body, []string{"id", "id"}, Node{}, nil, 0, 0)
	if err == nil {
		t.Fatal("err = nil, want the join body WHERE error")
	}
}

func TestSelectWithGroupedBodyShapes(t *testing.T) {
	t.Parallel()

	t.Run("empty grouped body falls back to COUNT(*)", func(t *testing.T) {
		t.Parallel()

		body := CTEBody{Kind: CTEBodyGrouped, Table: "orders"}

		q, _, err := SelectWith(sqlite.New(), "cats", false, body, []string{"count"}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if want := `WITH "cats" AS (SELECT COUNT(*) FROM "orders") SELECT "count" FROM "cats"`; q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("grouped body where and having", func(t *testing.T) {
		t.Parallel()

		body := CTEBody{
			Kind:      CTEBodyGrouped,
			Table:     "orders",
			GroupCols: []string{"category"},
			Where:     Node{Kind: KindBinary, Column: "region", Op: OpEq, Value: "west"},
			Having:    HavingNode{Kind: HavingLeaf, Agg: Aggregate{Func: AggCount}, Op: OpGt, Value: 2},
		}

		q, args, err := SelectWith(sqlite.New(), "cats", false, body, []string{"category"}, Node{}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `WITH "cats" AS (SELECT "category" FROM "orders" WHERE "region" = ? GROUP BY "category" HAVING COUNT(*) > ?) SELECT "category" FROM "cats"`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"west", 2}) {
			t.Fatalf("args = %#v, want [west 2]", args)
		}
	})

	t.Run("grouped body aggregate error propagates", func(t *testing.T) {
		t.Parallel()

		body := CTEBody{
			Kind:      CTEBodyGrouped,
			Table:     "orders",
			GroupCols: []string{"category"},
			Aggs:      []Aggregate{{Func: AggCount, DistinctArg: true}},
		}

		_, _, err := SelectWith(sqlite.New(), "cats", false, body, []string{"category"}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the aggregate error")
		}
	})

	t.Run("grouped body where error propagates", func(t *testing.T) {
		t.Parallel()

		body := CTEBody{
			Kind:      CTEBodyGrouped,
			Table:     "orders",
			GroupCols: []string{"category"},
			Where:     Node{Kind: KindBinary, Column: "region", Op: OpEqAny, Value: "west"},
		}

		_, _, err := SelectWith(sqlite.New(), "cats", false, body, []string{"category"}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})

	t.Run("grouped body having error propagates", func(t *testing.T) {
		t.Parallel()

		body := CTEBody{
			Kind:      CTEBodyGrouped,
			Table:     "orders",
			GroupCols: []string{"category"},
			Having:    HavingNode{Kind: HavingExpr},
		}

		_, _, err := SelectWith(sqlite.New(), "cats", false, body, []string{"category"}, Node{}, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the HAVING error")
		}
	})
}

func TestSelectWithJoinBodyOrderError(t *testing.T) {
	t.Parallel()

	body := CTEBody{
		Kind:         CTEBodyJoin,
		Table:        "users",
		Columns:      []string{"id"},
		RightTable:   "orders",
		RightColumns: []string{"id"},
		JoinType:     InnerJoin,
		ParentCol:    "id",
		ChildCol:     "user_id",
		Order:        []OrderTerm{{Func: &FuncExpr{}}},
	}

	_, _, err := SelectWith(sqlite.New(), "u_orders", false, body, []string{"id", "id"}, Node{}, nil, 0, 0)
	if err == nil {
		t.Fatal("err = nil, want the join body ORDER BY error")
	}
}
