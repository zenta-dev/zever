package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// nTuple builds a KindTuple node mirroring what the Tuple API produces:
// the LHS column list, an Op (In/NotIn membership or Eq..Lte row-value
// comparison) and the inner Subquery operand.
func nTuple(cols []string, op Op, sq Subquery) Node {
	return Node{Kind: KindTuple, Tuple: cols, Op: op, Value: sq}
}

func TestRenderTupleInSubquery(t *testing.T) {
	t.Parallel()
	n := nTuple([]string{"id", "quantity"}, OpIn,
		nSub("widget_orders", []string{"widget_id", "amount"},
			nBinary("widget_orders", "amount", OpGt, int64(20))))

	clause, args, err := renderExpr(sqlite.New(), n, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `("id", "quantity") IN (SELECT "widget_id", "amount" FROM "widget_orders" WHERE "amount" > ?)`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(20)}) {
		t.Fatalf("args = %#v, want the inner bound value only", args)
	}
}

func TestRenderTupleNotInSubquerySQLite(t *testing.T) {
	t.Parallel()
	n := nTuple([]string{"id", "quantity"}, OpNotIn,
		nSub("widget_orders", []string{"widget_id", "amount"},
			nBinary("widget_orders", "amount", OpGte, int64(25))))

	clause, args, err := renderExpr(sqlite.New(), n, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `("id", "quantity") NOT IN (SELECT "widget_id", "amount" FROM "widget_orders" WHERE "amount" >= ?)`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(25)}) {
		t.Fatalf("args = %#v, want the inner bound value only", args)
	}
}

// TestRenderTupleOuterArgOrder proves the tuple subquery continues the
// ENCLOSING statement's placeholder counter: with the plain outer predicate
// textually first, its argument is $1 and the inner tuple subquery's is $2.
func TestRenderTupleOuterArgOrder(t *testing.T) {
	t.Parallel()
	where := nCompound(CompoundAnd,
		nBinary("widgets", "quantity", OpGt, int64(15)),
		nTuple([]string{"id", "quantity"}, OpIn,
			nSub("widget_orders", []string{"widget_id", "amount"},
				nBinary("widget_orders", "amount", OpGt, int64(20)))))

	clause, args, err := renderExpr(fakePostgres{}, where, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `("quantity" > $1 AND ("id", "quantity") IN (SELECT "widget_id", "amount" FROM "widget_orders" WHERE "amount" > $2))`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(15), int64(20)}) {
		t.Fatalf("args = %#v, want outer arg before inner arg", args)
	}
}

// TestRenderTupleThenOuterArgOrder proves the counter keeps flowing AFTER a
// tuple subquery too: a following outer bound arg is numbered after the
// inner subquery's arguments.
func TestRenderTupleThenOuterArgOrder(t *testing.T) {
	t.Parallel()
	where := nCompound(CompoundAnd,
		nTuple([]string{"id", "quantity"}, OpIn,
			nSub("widget_orders", []string{"widget_id", "amount"},
				nBinary("widget_orders", "amount", OpGt, int64(20)))),
		nBinary("widgets", "quantity", OpGt, int64(15)))

	clause, args, err := renderExpr(fakePostgres{}, where, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `(("id", "quantity") IN (SELECT "widget_id", "amount" FROM "widget_orders" WHERE "amount" > $1) AND "quantity" > $2)`
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}

	if !reflect.DeepEqual(args, []any{int64(20), int64(15)}) {
		t.Fatalf("args = %#v, want inner arg before the following outer arg", args)
	}
}

func TestRenderTupleRowValueComparisonAllOps(t *testing.T) {
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
		n := nTuple([]string{"id", "quantity"}, o.op,
			nSub("widget_orders", []string{"widget_id", "amount"}, Node{}))

		clause, _, err := renderExpr(sqlite.New(), n, &argCounter{})
		if err != nil {
			t.Fatalf("op %d: err = %v, want nil", o.op, err)
		}

		want := `("id", "quantity") ` + o.want + ` (SELECT "widget_id", "amount" FROM "widget_orders")`
		if clause != want {
			t.Fatalf("op %d: clause = %q, want %q", o.op, clause, want)
		}
	}
}

func TestRenderTupleArityMismatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		node Node
	}{
		{
			"in too few inner",
			nTuple([]string{"id", "quantity"}, OpIn, nSub("widget_orders", []string{"widget_id"}, Node{})),
		},
		{
			"in too many inner",
			nTuple([]string{"id"}, OpIn, nSub("widget_orders", []string{"widget_id", "amount"}, Node{})),
		},
		{
			"comparison mismatch",
			nTuple([]string{"id", "quantity"}, OpEq, nSub("widget_orders", []string{"amount"}, Node{})),
		},
	}

	for _, tc := range tests {
		_, _, err := renderExpr(fakePostgres{}, tc.node, &argCounter{})
		if err == nil {
			t.Fatalf("%s: err = nil, want an arity-mismatch error", tc.name)
		}

		if !strings.Contains(err.Error(), "arity mismatch") {
			t.Fatalf("%s: err = %v, want it to mention arity mismatch", tc.name, err)
		}
	}
}

func TestRenderTupleZeroColumns(t *testing.T) {
	t.Parallel()
	_, _, err := renderExpr(fakePostgres{}, nTuple(nil, OpIn, nSub("widget_orders", []string{"widget_id"}, Node{})), &argCounter{})
	if err == nil {
		t.Fatalf("err = nil, want a zero-column tuple error")
	}

	if !strings.Contains(err.Error(), "at least one column") {
		t.Fatalf("err = %v, want it to mention at least one column", err)
	}
}

func TestRenderTupleNonSubqueryOperand(t *testing.T) {
	t.Parallel()
	_, _, err := renderExpr(fakePostgres{}, Node{Kind: KindTuple, Tuple: []string{"id"}, Op: OpIn, Value: []any{"x"}}, &argCounter{})
	if err == nil {
		t.Fatalf("err = nil, want a non-subquery-operand error")
	}

	if !strings.Contains(err.Error(), "subquery operand") {
		t.Fatalf("err = %v, want it to mention subquery operand", err)
	}
}

// TestRenderTupleInvalidOp proves an operator outside the membership/
// comparison set (e.g. Like) is a typed rendering error on a tuple node.
func TestRenderTupleInvalidOp(t *testing.T) {
	t.Parallel()
	_, _, err := renderExpr(sqlite.New(), nTuple([]string{"id"}, OpLike, nSub("widget_orders", []string{"widget_id"}, Node{})), &argCounter{})
	if err == nil {
		t.Fatalf("err = nil, want an invalid-tuple-operator error")
	}
}

// TestTupleShapesBypassCache proves tuple predicates render fresh every time
// -- like the other subquery shapes -- while still rendering byte-identically
// across repeats, extending the shape-cache equivalence contract.
func TestTupleShapesBypassCache(t *testing.T) {
	for _, d := range dialects(t) {
		sq := nSub("widget_orders", []string{"widget_id", "amount"},
			nBinary("widget_orders", "amount", OpGt, int64(10)))

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Select(d, "widgets", []string{"id"},
				nTuple([]string{"id", "quantity"}, OpIn, sq), nil, 0, 0)
		})

		renderTwiceNoHit(t, func() (string, []any, error) {
			return Count(d, "widgets", nTuple([]string{"id", "quantity"}, OpNotIn, sq))
		})
	}
}

// TestRenderTupleCorrelatedNoScope proves a tuple subquery's correlated WHERE
// marker fails closed when reached through the zero-scope renderExpr wrapper:
// there is no enclosing query to bind to, so a typed error is returned rather
// than wrong SQL. The positive binding case is covered by
// TestRenderTupleCorrelatedScoped below via a scoped Select.
func TestRenderTupleCorrelatedNoScope(t *testing.T) {
	t.Parallel()
	sq := nSub("widget_orders", []string{"widget_id", "amount"},
		nBinary("widget_orders", "widget_id", OpEq, OuterRef{Table: "widgets", Column: "id"}))

	n := nTuple([]string{"id", "quantity"}, OpIn, sq)

	_, _, err := renderExpr(fakePostgres{}, n, &argCounter{})
	if err == nil {
		t.Fatalf("err = nil, want a no-enclosing-query error")
	}

	if !strings.Contains(err.Error(), "has no enclosing query to bind to") {
		t.Fatalf("err = %v, want the no-enclosing-query error", err)
	}
}

// TestRenderTupleCorrelatedScoped proves a tuple subquery's correlated WHERE
// binds the directly enclosing single-table SELECT: the marker resolves
// through the same renderScope path as #399's correlated In-subquery, right
// next to an unrelated outer bound argument.
func TestRenderTupleCorrelatedScoped(t *testing.T) {
	t.Parallel()
	sq := nSub("widget_orders", []string{"widget_id", "amount"},
		nBinary("widget_orders", "widget_id", OpEq, OuterRef{Table: "widgets", Column: "id"}))

	n := nTuple([]string{"id", "quantity"}, OpIn, sq)

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id"},
		Node{Kind: KindCompound, Compound: CompoundAnd, Children: []Node{
			nBinary("widgets", "name", OpNeq, "Beta"),
			n,
		}}, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "widgets" WHERE ("name" != $1 AND ("id", "quantity") IN (SELECT "widget_id", "amount" FROM "widget_orders" WHERE "widget_id" = "widgets"."id"))`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"Beta"}) {
		t.Fatalf("args = %#v, want the outer bound value only", args)
	}
}
