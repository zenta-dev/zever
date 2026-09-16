package orm

import "testing"

type predT struct{}

func TestPredicateIsSetRender(t *testing.T) {
	var p Predicate[predT]
	if p.IsSet() {
		t.Fatalf("zero Predicate.IsSet() = true, want false")
	}

	if p.Render().Kind != NNone {
		t.Fatalf("zero Predicate.Render().Kind = %v, want NNone", p.Render().Kind)
	}

	c := NewColumn[predT, string]("t", "c")
	set := c.Eq("x")

	if !set.IsSet() {
		t.Fatalf("c.Eq(\"x\").IsSet() = false, want true")
	}

	n := set.Render()
	if n.Kind != NBinary || n.Op != Eq || n.Column != "c" || n.Table != "t" || n.Value != "x" {
		t.Fatalf("Render() = %+v, unexpected", n)
	}
}

func TestAndOrCollapsing(t *testing.T) {
	c := NewColumn[predT, string]("t", "c")

	if got := And[predT](); got.IsSet() {
		t.Fatalf("And() with zero args IsSet() = true, want false (unset)")
	}

	if got := Or[predT](); got.IsSet() {
		t.Fatalf("Or() with zero args IsSet() = true, want false (unset)")
	}

	one := c.Eq("a")
	if got := And(one); got.Render().Kind != NBinary {
		t.Fatalf("And(single) should return the predicate unwrapped, got Kind=%v", got.Render().Kind)
	}

	two := And(c.Eq("a"), c.Eq("b"))
	n := two.Render()

	if n.Kind != NCompound || n.Compound != CAnd || len(n.Children) != 2 {
		t.Fatalf("And(a,b) = %+v, want a 2-child NCompound/CAnd", n)
	}

	orN := Or(c.Eq("a"), c.Eq("b")).Render()
	if orN.Kind != NCompound || orN.Compound != COr || len(orN.Children) != 2 {
		t.Fatalf("Or(a,b) = %+v, want a 2-child NCompound/COr", orN)
	}

	// Unset predicates are skipped when combining.
	mixed := And(Predicate[predT]{}, c.Eq("a"))
	if mixed.Render().Kind != NBinary {
		t.Fatalf("And(unset, set) should collapse to the single set predicate, got Kind=%v", mixed.Render().Kind)
	}
}

func TestNot(t *testing.T) {
	c := NewColumn[predT, string]("t", "c")

	if got := Not(Predicate[predT]{}); got.IsSet() {
		t.Fatalf("Not(unset).IsSet() = true, want false")
	}

	n := Not(c.Eq("a")).Render()
	if n.Kind != NCompound || n.Compound != CNot || len(n.Children) != 1 {
		t.Fatalf("Not(set) = %+v, want a 1-child NCompound/CNot", n)
	}
}

// TestExistsNodeShape pins the erased shape: Exists builds an NSubquery
// node carrying the inner query snapshot, and NotExists wraps it in a
// negation.
func TestExistsNodeShape(t *testing.T) {
	inner := From(widgets).Where(widgetQty.Gt(5))

	n := Exists[widget](inner).Render()
	if n.Kind != NSubquery {
		t.Fatalf("Exists Kind = %v, want NSubquery", n.Kind)
	}

	if _, ok := n.Value.(subquery); !ok {
		t.Fatalf("Exists Value type %T, want a subquery", n.Value)
	}

	neg := NotExists[widget](inner).Render()
	if neg.Kind != NCompound || neg.Compound != CNot || len(neg.Children) != 1 {
		t.Fatalf("NotExists = %+v, want a 1-child NCompound/CNot", neg)
	}

	if neg.Children[0].Kind != NSubquery {
		t.Fatalf("NotExists child Kind = %v, want NSubquery", neg.Children[0].Kind)
	}
}

// TestExistsRoundTrip documents orm's non-correlated EXISTS semantics: the
// inner query is evaluated independently (its Where carries no
// Outer/OuterNullable reference), so its truth value is the same for every
// outer row. A satisfied inner query thus flags all outer rows, not just
// the matching widget.
func TestExistsRoundTrip(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	matched := From(widgetOrders).Where(orderWidgetID.Eq("w2"))
	satisfied, err := From(widgets).
		Where(Exists[widget](matched)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, satisfied, "w1", "w2", "w3")

	none := From(widgetOrders).Where(orderWidgetID.Eq("missing"))
	unsatisfied, err := From(widgets).
		Where(Exists[widget](none)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All unsatisfied: %v", err)
	}

	if len(unsatisfied) != 0 {
		t.Fatalf("unsatisfied EXISTS returned rows %v, want none", widgetIDs(unsatisfied))
	}
}

func TestNotExistsRoundTrip(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	// No order amounts above 100 exist, so the existential is FALSE for
	// every outer row; NOT flips the whole filter to TRUE.
	inner := From(widgetOrders).Where(woAmount.Gt(100))

	got, err := From(widgets).
		Where(NotExists[widget](inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1", "w2", "w3")

	// When the existential IS satisfied, NOT EXISTS returns no rows.
	satisfied := From(widgetOrders).Where(orderID.Eq("o1"))
	blocked, err := From(widgets).
		Where(NotExists[widget](satisfied)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All blocked: %v", err)
	}

	if len(blocked) != 0 {
		t.Fatalf("satisfied NOT EXISTS returned rows %v, want none", widgetIDs(blocked))
	}
}
