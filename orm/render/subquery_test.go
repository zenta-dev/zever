package render

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// subquery-test shapes below mirror what orm's fluent API builds: an
// In-subquery node (a KindIn node whose Value is a Subquery), an EXISTS
// node (a KindSubquery node), a NOT IN node (a KindIn node with OpNotIn),
// and a scalar comparison (a KindBinary node whose Value is a Subquery).

func nSub(table string, columns []string, where Node) Subquery {
	return Subquery{Table: table, Columns: columns, Where: where}
}

func TestRenderInSubquery(t *testing.T) {
	t.Parallel()
	where := Node{
		Kind:   KindIn,
		Table:  "widgets",
		Column: "id",
		Op:     OpIn,
		Value: nSub("widget_orders", []string{"widget_id"},
			nBinary("widget_orders", "amount", OpGt, int64(20))),
	}

	clause, args, err := renderExpr(sqlite.New(), where, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `"id" IN (SELECT "widget_id" FROM "widget_orders" WHERE "amount" > ?)`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(20)}) {
		t.Fatalf("args = %#v, want the inner bound value only", args)
	}
}

func TestRenderInSubqueryPostgresArgOrder(t *testing.T) {
	t.Parallel()
	// The outer predicate's own bound value must be numbered BEFORE the
	// inner subquery's -- the proof that the subquery renders through the
	// enclosing statement's placeholder counter (outer args before inner
	// args both in text order and in the returned args slice, for $N
	// dialects).
	where := nCompound(CompoundAnd,
		nBinary("widgets", "quantity", OpGt, int64(15)),
		Node{
			Kind:   KindIn,
			Table:  "widgets",
			Column: "id",
			Op:     OpIn,
			Value: nSub("widget_orders", []string{"widget_id"},
				nBinary("widget_orders", "amount", OpGt, int64(20))),
		})

	clause, args, err := renderExpr(fakePostgres{}, where, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `("quantity" > $1 AND "id" IN (SELECT "widget_id" FROM "widget_orders" WHERE "amount" > $2))`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(15), int64(20)}) {
		t.Fatalf("args = %#v, want outer arg before inner arg", args)
	}
}

func TestRenderNotInSubquery(t *testing.T) {
	t.Parallel()
	where := Node{
		Kind:   KindIn,
		Table:  "widgets",
		Column: "id",
		Op:     OpNotIn,
		Value: nSub("widget_orders", []string{"widget_id"},
			nBinary("widget_orders", "amount", OpGte, int64(25))),
	}

	clause, args, err := renderExpr(sqlite.New(), where, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `"id" NOT IN (SELECT "widget_id" FROM "widget_orders" WHERE "amount" >= ?)`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(25)}) {
		t.Fatalf("args = %#v, want the inner bound value only", args)
	}
}

func TestRenderExists(t *testing.T) {
	t.Parallel()
	n := Node{Kind: KindSubquery, Value: nSub("widget_orders", []string{"widget_id"},
		nBinary("widget_orders", "widget_id", OpEq, "w1"))}

	clause, args, err := renderExpr(sqlite.New(), n, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `EXISTS (SELECT "widget_id" FROM "widget_orders" WHERE "widget_id" = ?)`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{"w1"}) {
		t.Fatalf("args = %#v, want the inner bound value only", args)
	}
}

func TestRenderNotExists(t *testing.T) {
	t.Parallel()
	// NotExists is Not(Exists(inner)) -- a NOT compound over the EXISTS
	// subquery; the counter continues across the nesting.
	n := nCompound(CompoundNot, Node{Kind: KindSubquery,
		Value: nSub("widget_orders", []string{"widget_id"},
			nBinary("widget_orders", "amount", OpGt, int64(5)))})

	clause, args, err := renderExpr(fakePostgres{}, n, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `NOT (EXISTS (SELECT "widget_id" FROM "widget_orders" WHERE "amount" > $1))`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(5)}) {
		t.Fatalf("args = %#v, want the inner bound value", args)
	}
}

func TestRenderScalarSubquery(t *testing.T) {
	t.Parallel()
	n := Node{Kind: KindBinary, Table: "widgets", Column: "quantity", Op: OpEq,
		Value: nSub("widget_orders", []string{"amount"},
			nBinary("widget_orders", "id", OpEq, "o1"))}

	clause, args, err := renderExpr(sqlite.New(), n, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `"quantity" = (SELECT "amount" FROM "widget_orders" WHERE "id" = ?)`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{"o1"}) {
		t.Fatalf("args = %#v, want the inner bound value", args)
	}
}

func TestRenderScalarSubqueryAllOps(t *testing.T) {
	t.Parallel()
	ops := []struct {
		op   Op
		want string
	}{
		{OpEq, "="},
		{OpNeq, "!="},
		{OpGt, ">"},
		{OpGte, ">="},
		{OpLt, "<"},
		{OpLte, "<="},
	}

	for _, o := range ops {
		n := Node{Kind: KindBinary, Table: "widgets", Column: "quantity", Op: o.op,
			Value: nSub("widget_orders", []string{"amount"}, Node{})}

		clause, _, err := renderExpr(sqlite.New(), n, &argCounter{})
		if err != nil {
			t.Fatalf("op %d: err = %v, want nil", o.op, err)
		}

		want := fmt.Sprintf(`"quantity" %s (SELECT "amount" FROM "widget_orders")`, o.want)
		if clause != want {
			t.Fatalf("op %d: clause = %q, want %q", o.op, clause, want)
		}
	}
}

func TestRenderInnerLimitOffsetNumbering(t *testing.T) {
	t.Parallel()
	// The subquery's own LIMIT/OFFSET placeholders continue the enclosing
	// counter after the inner WHERE's.
	where := Node{Kind: KindIn, Table: "widgets", Column: "id", Op: OpIn,
		Value: Subquery{
			Table:   "widget_orders",
			Columns: []string{"widget_id"},
			Where:   nBinary("widget_orders", "amount", OpGt, int64(5)),
			Limit:   1,
			Offset:  2,
		}}

	clause, args, err := renderExpr(fakePostgres{}, where, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `"id" IN (SELECT "widget_id" FROM "widget_orders" WHERE "amount" > $1 LIMIT $2 OFFSET $3)`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(5), 1, 2}) {
		t.Fatalf("args = %#v, want [5, 1, 2]", args)
	}
}

func TestRenderNestedSubqueryArgOrder(t *testing.T) {
	t.Parallel()
	// Outer -> In-subquery -> inner In-subquery: placeholders are numbered
	// depth-first in text order through the shared counter.
	where := nCompound(CompoundAnd,
		nBinary("widgets", "quantity", OpGt, int64(5)),
		Node{Kind: KindIn, Table: "widgets", Column: "id", Op: OpIn,
			Value: Subquery{
				Table:   "widget_orders",
				Columns: []string{"widget_id"},
				Where: nCompound(CompoundAnd,
					nBinary("widget_orders", "amount", OpGt, int64(10)),
					Node{Kind: KindIn, Table: "widget_orders", Column: "widget_id", Op: OpIn,
						Value: nSub("order_meta", []string{"widget_id"},
							nBinary("order_meta", "active", OpEq, true))}),
			}})

	clause, args, err := renderExpr(fakePostgres{}, where, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `("quantity" > $1 AND "id" IN (SELECT "widget_id" FROM "widget_orders" WHERE ("amount" > $2 AND "widget_id" IN (SELECT "widget_id" FROM "order_meta" WHERE "active" = $3))))`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(5), int64(10), true}) {
		t.Fatalf("args = %#v, want [5, 10, true] in depth-first text order", args)
	}
}

func TestSelectFullStatementWithSubquery(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindIn, Table: "widgets", Column: "id", Op: OpIn,
		Value: nSub("widget_orders", []string{"widget_id"},
			nBinary("widget_orders", "amount", OpGte, int64(25)))}

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id", "name"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id", "name" FROM "widgets" WHERE "id" IN (SELECT "widget_id" FROM "widget_orders" WHERE "amount" >= $1)`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{int64(25)}) {
		t.Fatalf("args = %#v, want [25]", args)
	}
}

func TestRenderMultiColumnSubqueryRejected(t *testing.T) {
	t.Parallel()
	multi := nSub("widget_orders", []string{"widget_id", "amount"}, Node{})

	tests := []struct {
		name string
		node Node
	}{
		{"in", Node{Kind: KindIn, Table: "widgets", Column: "id", Op: OpIn, Value: multi}},
		{"not-in", Node{Kind: KindIn, Table: "widgets", Column: "id", Op: OpNotIn, Value: multi}},
		{"scalar", Node{Kind: KindBinary, Table: "widgets", Column: "quantity", Op: OpEq, Value: multi}},
	}

	for _, tc := range tests {
		_, _, err := renderExpr(fakePostgres{}, tc.node, &argCounter{})
		if err == nil {
			t.Fatalf("%s: err = nil, want a single-column projection error", tc.name)
		}
	}
}

func TestRenderExistsMalformedValue(t *testing.T) {
	t.Parallel()
	_, _, err := renderExpr(fakePostgres{}, Node{Kind: KindSubquery, Value: "not-a-subquery"}, &argCounter{})
	if err == nil {
		t.Fatalf("err = nil, want a malformed-subquery error")
	}
}

// TestSubqueryShapesBypassCache proves subquery predicates (In-subquery,
// EXISTS, scalar comparison) render fresh every time -- never cached, never
// hit -- while still rendering byte-identically across repeats, extending
// the shape-cache equivalence contract the repo enforces.
func TestSubqueryShapesBypassCache(t *testing.T) {
	for _, d := range dialects(t) {
		sq := nSub("orders", []string{"widget_id"}, nBinary("orders", "amount", OpGt, int64(10)))

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Select(d, "users", []string{"id"},
				Node{Kind: KindIn, Table: "users", Column: "id", Op: OpIn, Value: sq}, nil, 0, 0)
		})

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Select(d, "users", []string{"id"}, Node{Kind: KindSubquery, Value: sq}, nil, 0, 0)
		})

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Select(d, "users", []string{"id"},
				nBinary("users", "id", OpEq, sq), nil, 0, 0)
		})

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Count(d, "users", Node{Kind: KindIn, Table: "users", Column: "id", Op: OpIn, Value: sq})
		})
	}
}
