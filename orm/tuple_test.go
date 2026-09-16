package orm

import (
	"reflect"
	"strings"
	"testing"
)

// TestTupleNodeShape pins the erased Node shape: a tuple predicate is an
// NTuple node carrying the LHS column list in Tuple, the membership/
// comparison Op, and the inner subquery in Value.
func TestTupleNodeShape(t *testing.T) {
	inner := From(widgetOrders).Columns(orderWidgetID.Col(), woAmount.Col())

	tests := []struct {
		name string
		pred Predicate[widget]
		op   Op
	}{
		{"in", NewTuple(widgetID.Col(), widgetQty.Col()).In(inner), In},
		{"not-in", NewTuple(widgetID.Col(), widgetQty.Col()).NotIn(inner), NotIn},
		{"eq", NewTuple(widgetID.Col(), widgetQty.Col()).Eq(inner), Eq},
		{"lte", NewTuple(widgetID.Col(), widgetQty.Col()).Lte(inner), Lte},
	}

	for _, tc := range tests {
		n := tc.pred.Render()

		if n.Kind != NTuple {
			t.Fatalf("%s: Kind = %d, want NTuple", tc.name, n.Kind)
		}

		if !reflect.DeepEqual(n.Tuple, []string{"id", "quantity"}) {
			t.Fatalf("%s: Tuple = %v, want [id quantity]", tc.name, n.Tuple)
		}

		if n.Op != tc.op {
			t.Fatalf("%s: Op = %d, want %d", tc.name, n.Op, tc.op)
		}

		if _, ok := n.Value.(subquery); !ok {
			t.Fatalf("%s: Value type %T, want a subquery", tc.name, n.Value)
		}
	}
}

// TestTupleInSubqueryRoundTrip returns widgets whose (id, quantity) pair
// appears among the (widget_id, amount) pairs of orders above 5: only w1's
// (w1, 10) pair is present -- w2's (w2, 20) and w3's (w3, 30) are not, even
// though their ids alone ARE in the inner id set.
func TestTupleInSubqueryRoundTrip(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgetOrders).Where(woAmount.Gt(5)).Columns(orderWidgetID.Col(), woAmount.Col())

	got, err := From(widgets).
		Where(NewTuple(widgetID.Col(), widgetQty.Col()).In(inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1")
}

func TestTupleNotInSubqueryRoundTrip(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgetOrders).Where(woAmount.Gt(5)).Columns(orderWidgetID.Col(), woAmount.Col())

	got, err := From(widgets).
		Where(NewTuple(widgetID.Col(), widgetQty.Col()).NotIn(inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w2", "w3")
}

// TestTupleInSubqueryComposesCompound nests a tuple predicate inside Or and
// Not alongside plain predicates: the 2-column tuple matches w1, the name
// disjunct adds w3, and the negation keeps w2/w3.
func TestTupleInSubqueryComposesCompound(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	tuples := From(widgetOrders).Where(woAmount.Gt(5)).Columns(orderWidgetID.Col(), woAmount.Col())
	pred := NewTuple(widgetID.Col(), widgetQty.Col()).In(tuples)

	got, err := From(widgets).
		Where(Or(pred, widgetName.Eq("Gamma"))).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1", "w3")

	negated, err := From(widgets).
		Where(Not(pred)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All negated: %v", err)
	}

	wantIDs(t, negated, "w2", "w3")
}

// TestTupleRowValueComparisonRoundTrip exercises the row-value comparison
// operators against a single-row, two-column subquery. The inner row is
// (w1, 10); SQL compares (id, quantity) lexicographically against it.
func TestTupleRowValueComparisonRoundTrip(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	oneRow := func() Query[widgetOrder, *widgetOrder] {
		return From(widgetOrders).Where(orderID.Eq("o1")).Columns(orderWidgetID.Col(), woAmount.Col())
	}

	tests := []struct {
		name string
		pred Predicate[widget]
		want []string
	}{
		{"eq", NewTuple(widgetID.Col(), widgetQty.Col()).Eq(oneRow()), []string{"w1"}},
		{"neq", NewTuple(widgetID.Col(), widgetQty.Col()).Neq(oneRow()), []string{"w2", "w3"}},
		{"gt", NewTuple(widgetID.Col(), widgetQty.Col()).Gt(oneRow()), []string{"w2", "w3"}},
		{"gte", NewTuple(widgetID.Col(), widgetQty.Col()).Gte(oneRow()), []string{"w1", "w2", "w3"}},
		{"lt", NewTuple(widgetID.Col(), widgetQty.Col()).Lt(oneRow()), nil},
		{"lte", NewTuple(widgetID.Col(), widgetQty.Col()).Lte(oneRow()), []string{"w1"}},
	}

	for _, tc := range tests {
		got, err := From(widgets).Where(tc.pred).OrderBy(widgetID.Asc()).All(ctx, conn)
		if err != nil {
			t.Fatalf("%s: All: %v", tc.name, err)
		}

		wantIDs(t, got, tc.want...)
	}
}

// TestTupleOuterBoundArgRoundTrip combines an outer bound-value predicate
// with a tuple subquery that binds its own inner value. The outer argument
// ("Beta") must be bound before the inner one (5); a swapped order would make
// the name comparison always-true and the integer amount comparison
// always-false, yielding no rows.
func TestTupleOuterBoundArgRoundTrip(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgetOrders).Where(woAmount.Gt(5)).Columns(orderWidgetID.Col(), woAmount.Col())

	got, err := From(widgets).
		Where(And(
			widgetName.Neq("Beta"),
			NewTuple(widgetID.Col(), widgetQty.Col()).In(inner),
		)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1")
}

// TestTupleCorrelatedRoundTrip proves correlation works inside a tuple
// subquery exactly as it does for the In-subquery: the inner query
// selects only THIS widget's own (widget_id, amount) pairs, so w1's
// (w1, 10) pair matches, w2/w3's do not, and w4 (no orders) does not.
func TestTupleCorrelatedRoundTrip(t *testing.T) {
	ctx, conn := newCorrelatedDB(t)

	inner := From(widgetOrders).
		Where(orderWidgetID.EqOuter(Outer(widgetID))).
		Columns(orderWidgetID.Col(), woAmount.Col())

	got, err := From(widgets).
		Where(NewTuple(widgetID.Col(), widgetQty.Col()).In(inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1")
}

// TestTupleArityMismatchErrors proves the tuple column count must equal the
// subquery's projected column count, reported as a typed rendering error at
// execution time rather than invalid SQL.
func TestTupleArityMismatchErrors(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	oneColumn := From(widgetOrders).Columns(orderWidgetID.Col())

	_, err := From(widgets).
		Where(NewTuple(widgetID.Col(), widgetQty.Col()).In(oneColumn)).
		All(ctx, conn)
	if err == nil {
		t.Fatalf("All with an arity mismatch succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "arity mismatch") {
		t.Fatalf("err = %v, want it to mention arity mismatch", err)
	}
}

// TestTupleZeroColumnsErrors proves an empty tuple is rejected rather than
// rendering a bare `() IN (...)`.
func TestTupleZeroColumnsErrors(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgetOrders).Columns(orderWidgetID.Col(), woAmount.Col())

	_, err := From(widgets).
		Where(NewTuple[widget]().In(inner)).
		All(ctx, conn)
	if err == nil {
		t.Fatalf("All with a zero-column tuple succeeded, want an error")
	}

	if !strings.Contains(err.Error(), "at least one column") {
		t.Fatalf("err = %v, want it to mention at least one column", err)
	}
}
