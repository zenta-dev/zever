package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestJoinTypeKeywords pins every JOIN keyword, including the INNER fallback
// for an out-of-range join type.
func TestJoinTypeKeywords(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		jt   JoinType
		want string
	}{
		{"inner", InnerJoin, "INNER JOIN"},
		{"left", LeftJoin, "LEFT JOIN"},
		{"right", RightJoin, "RIGHT JOIN"},
		{"full", FullJoin, "FULL JOIN"},
		{"unknown defaults to inner", JoinType(99), "INNER JOIN"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.jt.keyword(); got != tc.want {
				t.Fatalf("keyword() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSelectJoinLimitOffset pins the LIMIT/OFFSET tail on a projecting join
// and its Postgres placeholder numbering after the WHERE args. Both WHERE
// sides are set, so the join's AND-combination path renders too.
func TestSelectJoinLimitOffset(t *testing.T) {
	t.Parallel()

	whereLeft := nBinary("widgets", "active", OpEq, true)
	whereRight := nBinary("widget_orders", "note", OpEq, "x")

	q, args, err := SelectJoin(postgres.New(), LeftJoin,
		"widgets", []string{"id"},
		"widget_orders", []string{"id"},
		"id", "widget_id",
		whereLeft, whereRight,
		[]OrderTerm{{Column: "id"}}, nil, 5, 10)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "widgets"."id", "widget_orders"."id" FROM "widgets" LEFT JOIN "widget_orders" ON "widgets"."id" = "widget_orders"."widget_id" WHERE ("widgets"."active" = $1 AND "widget_orders"."note" = $2) ORDER BY "widgets"."id" ASC LIMIT $3 OFFSET $4`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{true, "x", 5, 10}) {
		t.Fatalf("args = %#v, want [true x 5 10]", args)
	}
}

// TestSelectJoinNoWhere renders no WHERE clause when both sides are unset.
func TestSelectJoinNoWhere(t *testing.T) {
	t.Parallel()

	q, _, err := SelectJoin(sqlite.New(), InnerJoin,
		"widgets", []string{"id"},
		"widget_orders", []string{"id"},
		"id", "widget_id",
		Node{}, Node{}, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "widgets"."id", "widget_orders"."id" FROM "widgets" INNER JOIN "widget_orders" ON "widgets"."id" = "widget_orders"."widget_id"`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

func TestSelectJoinErrorPaths(t *testing.T) {
	t.Parallel()

	bad := Node{Kind: KindBinary, Column: "active", Op: OpEqAny, Value: true}

	t.Run("where error propagates", func(t *testing.T) {
		t.Parallel()

		_, _, err := SelectJoin(sqlite.New(), InnerJoin,
			"widgets", []string{"id"},
			"widget_orders", []string{"id"},
			"id", "widget_id",
			bad, Node{}, nil, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})

	t.Run("order error propagates", func(t *testing.T) {
		t.Parallel()

		badOrder := []OrderTerm{{Func: &FuncExpr{}}}

		_, _, err := SelectJoin(sqlite.New(), InnerJoin,
			"widgets", []string{"id"},
			"widget_orders", []string{"id"},
			"id", "widget_id",
			Node{}, Node{}, badOrder, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the ORDER BY error")
		}
	})
}

func TestSelectJoin3TailAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("limit offset tail", func(t *testing.T) {
		t.Parallel()

		q, args, err := SelectJoin3(postgres.New(), InnerJoin,
			"a", []string{"id"}, "b", []string{"id"}, "c", []string{"id"},
			"id", "a_id", "id", "b_id",
			nBinary("a", "active", OpEq, true), Node{}, Node{},
			nil, nil, nil, 5, 10)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "a"."id", "b"."id", "c"."id" FROM "a" INNER JOIN "b" ON "a"."id" = "b"."a_id" INNER JOIN "c" ON "b"."id" = "c"."b_id" WHERE "a"."active" = $1 LIMIT $2 OFFSET $3`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{true, 5, 10}) {
			t.Fatalf("args = %#v, want [true 5 10]", args)
		}
	})

	t.Run("where error propagates", func(t *testing.T) {
		t.Parallel()

		bad := Node{Kind: KindBinary, Column: "active", Op: OpEqAny, Value: true}

		_, _, err := SelectJoin3(sqlite.New(), InnerJoin,
			"a", []string{"id"}, "b", []string{"id"}, "c", []string{"id"},
			"id", "a_id", "id", "b_id",
			bad, Node{}, Node{}, nil, nil, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})

	t.Run("order error propagates", func(t *testing.T) {
		t.Parallel()

		badOrder := []OrderTerm{{Func: &FuncExpr{}}}

		_, _, err := SelectJoin3(sqlite.New(), InnerJoin,
			"a", []string{"id"}, "b", []string{"id"}, "c", []string{"id"},
			"id", "a_id", "id", "b_id",
			Node{}, Node{}, Node{}, badOrder, nil, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the ORDER BY error")
		}
	})
}

func TestSelectLateralOffsetAndErrors(t *testing.T) {
	t.Parallel()

	inner := func() Subquery {
		return nSub("widget_orders", []string{"id"},
			nBinary("widget_orders", "amount", OpGt, int64(20)))
	}

	t.Run("offset tail", func(t *testing.T) {
		t.Parallel()

		q, args, err := SelectLateral(postgres.New(), true,
			"widgets", []string{"id"}, inner(), "recent",
			Node{}, nil, nil, 5, 10)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if !strings.Contains(q, "OFFSET $") {
			t.Fatalf("query = %q, want an OFFSET clause", q)
		}

		if len(args) != 3 || args[1] != 5 || args[2] != 10 {
			t.Fatalf("args = %#v, want [20 5 10]", args)
		}
	})

	t.Run("where error propagates", func(t *testing.T) {
		t.Parallel()

		bad := Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: "w1"}

		_, _, err := SelectLateral(sqlite.New(), true,
			"widgets", []string{"id"}, inner(), "recent",
			bad, nil, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})

	t.Run("order error propagates", func(t *testing.T) {
		t.Parallel()

		badOrder := []OrderTerm{{Func: &FuncExpr{}}}

		_, _, err := SelectLateral(sqlite.New(), true,
			"widgets", []string{"id"}, inner(), "recent",
			Node{}, badOrder, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the ORDER BY error")
		}
	})
}
