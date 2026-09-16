package orm

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
)

type colT struct{}

func TestColumnComparisons(t *testing.T) {
	c := NewColumn[colT, int64]("t", "n")

	cases := []struct {
		name string
		pred Predicate[colT]
		op   Op
	}{
		{"Eq", c.Eq(1), Eq},
		{"Neq", c.Neq(1), Neq},
		{"Gt", c.Gt(1), Gt},
		{"Gte", c.Gte(1), Gte},
		{"Lt", c.Lt(1), Lt},
		{"Lte", c.Lte(1), Lte},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.pred.Render()
			if n.Op != tc.op || n.Kind != NBinary {
				t.Fatalf("%s: node = %+v", tc.name, n)
			}
		})
	}
}

func TestColumnIn(t *testing.T) {
	c := NewColumn[colT, int64]("t", "n")

	n := c.In(1, 2, 3).Render()
	if n.Kind != NIn || n.Op != In {
		t.Fatalf("In: node = %+v", n)
	}

	vs, ok := n.Value.([]any)
	if !ok || len(vs) != 3 {
		t.Fatalf("In: Value = %#v, want []any of len 3", n.Value)
	}
}

func TestColumnBetween(t *testing.T) {
	c := NewColumn[colT, int64]("t", "n")

	n := c.Between(1, 10).Render()
	if n.Kind != NBetween || n.Op != Between {
		t.Fatalf("Between: node = %+v", n)
	}

	pair, ok := n.Value.([2]any)
	if !ok || pair[0] != int64(1) || pair[1] != int64(10) {
		t.Fatalf("Between: Value = %#v", n.Value)
	}
}

func TestColumnOrder(t *testing.T) {
	c := NewColumn[colT, int64]("t", "n")

	if got := c.Asc(); got.Column.Name() != "n" || got.Desc {
		t.Fatalf("Asc() = %+v", got)
	}

	if got := c.Desc(); got.Column.Name() != "n" || !got.Desc {
		t.Fatalf("Desc() = %+v", got)
	}
}

func TestColumnCol(t *testing.T) {
	c := NewColumn[colT, int64]("t", "n")
	if got := c.Col().Name(); got != "n" {
		t.Fatalf("Col().Name() = %q, want %q", got, "n")
	}
}

func TestColumnTableName(t *testing.T) {
	c := NewColumn[colT, int64]("t", "n")
	if c.Table() != "t" || c.Name() != "n" {
		t.Fatalf("Table()/Name() = (%q, %q), want (t, n)", c.Table(), c.Name())
	}
}

func TestContainsStartsWithEndsWithEscaping(t *testing.T) {
	c := NewColumn[colT, string]("t", "s")

	tests := []struct {
		name string
		pred Predicate[colT]
		want string
	}{
		{"Contains", Contains(c, "50%_off\\"), "%50\\%\\_off\\\\%"},
		{"StartsWith", StartsWith(c, "50%_off\\"), "50\\%\\_off\\\\%"},
		{"EndsWith", EndsWith(c, "50%_off\\"), "%50\\%\\_off\\\\"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.pred.Render()
			if n.Kind != NLike || n.Op != Like {
				t.Fatalf("%s: node = %+v", tc.name, n)
			}

			if n.Value != tc.want {
				t.Fatalf("%s: Value = %q, want %q", tc.name, n.Value, tc.want)
			}
		})
	}
}

func TestLikeRawNoEscaping(t *testing.T) {
	c := NewColumn[colT, string]("t", "s")

	n := LikeRaw(c, "50%_off").Render()
	if n.Kind != NLike || n.Value != "50%_off" {
		t.Fatalf("LikeRaw: node = %+v, want unescaped pattern", n)
	}
}

func TestNullableColumn(t *testing.T) {
	c := NewNullableColumn[colT, string]("t", "bio")

	if n := c.Eq("x").Render(); n.Op != Eq || n.Column != "bio" {
		t.Fatalf("Eq: node = %+v", n)
	}

	if n := c.IsNull().Render(); n.Op != IsNull {
		t.Fatalf("IsNull: node = %+v", n)
	}

	if n := c.IsNotNull().Render(); n.Op != IsNotNull {
		t.Fatalf("IsNotNull: node = %+v", n)
	}

	if got := c.Asc(); got.Column.Name() != "bio" {
		t.Fatalf("Asc() = %+v", got)
	}

	if got := c.Desc(); !got.Desc {
		t.Fatalf("Desc() = %+v", got)
	}

	if got := c.Col().Name(); got != "bio" {
		t.Fatalf("Col().Name() = %q", got)
	}

	if got := c.In("a", "b").Render(); got.Kind != NIn {
		t.Fatalf("In: node = %+v", got)
	}

	if got := c.Between("a", "z").Render(); got.Kind != NBetween {
		t.Fatalf("Between: node = %+v", got)
	}

	if got := c.Neq("x").Render(); got.Op != Neq {
		t.Fatalf("Neq: node = %+v", got)
	}

	if got := c.Gt("x").Render(); got.Op != Gt {
		t.Fatalf("Gt: node = %+v", got)
	}

	if got := c.Gte("x").Render(); got.Op != Gte {
		t.Fatalf("Gte: node = %+v", got)
	}

	if got := c.Lt("x").Render(); got.Op != Lt {
		t.Fatalf("Lt: node = %+v", got)
	}

	if got := c.Lte("x").Render(); got.Op != Lte {
		t.Fatalf("Lte: node = %+v", got)
	}

	if c.Table() != "t" || c.Name() != "bio" {
		t.Fatalf("Table()/Name() = (%q, %q), want (t, bio)", c.Table(), c.Name())
	}
}

func TestNullableColumnAssignments(t *testing.T) {
	c := NewNullableColumn[colT, string]("t", "bio")

	a := c.SetValue("hi")
	if a.Column.Name() != "bio" || a.Value != "hi" {
		t.Fatalf("SetValue: %+v", a)
	}

	a2 := c.SetNull()
	if a2.Column.Name() != "bio" || a2.Value != nil {
		t.Fatalf("SetNull: %+v", a2)
	}
}

// TestColumnOuterComparisons pins the correlated-marker shape: every
// *Outer method builds a binary comparison whose Value is the OuterRef
// marker untouched (not a bound value), so the renderer resolves it
// against the enclosing query at execution time.
func TestColumnOuterComparisons(t *testing.T) {
	c := NewColumn[colT, string]("t", "c")
	outer := Outer(NewColumn[colT, string]("o", "c"))

	cases := []struct {
		name string
		pred Predicate[colT]
		op   Op
	}{
		{"EqOuter", c.EqOuter(outer), Eq},
		{"NeqOuter", c.NeqOuter(outer), Neq},
		{"GtOuter", c.GtOuter(outer), Gt},
		{"GteOuter", c.GteOuter(outer), Gte},
		{"LtOuter", c.LtOuter(outer), Lt},
		{"LteOuter", c.LteOuter(outer), Lte},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.pred.Render()
			if n.Kind != NBinary || n.Op != tc.op {
				t.Fatalf("%s: node = %+v", tc.name, n)
			}

			ref, ok := n.Value.(OuterRef[colT, string])
			if !ok {
				t.Fatalf("%s: Value type %T, want an OuterRef marker", tc.name, n.Value)
			}

			if ref.outerRefTable() != "o" || ref.outerRefName() != "c" {
				t.Fatalf("%s: marker = %v/%v, want o/c", tc.name, ref.outerRefTable(), ref.outerRefName())
			}
		})
	}
}

// TestNullableColumnOuterComparisons covers the NullableColumn side of the
// correlated-marker surface, including a marker built from a nullable
// column.
func TestNullableColumnOuterComparisons(t *testing.T) {
	c := NewNullableColumn[colT, string]("t", "c")
	outer := OuterNullable(NewNullableColumn[colT, string]("o", "c"))

	cases := []struct {
		name string
		pred Predicate[colT]
		op   Op
	}{
		{"EqOuter", c.EqOuter(outer), Eq},
		{"NeqOuter", c.NeqOuter(outer), Neq},
		{"GtOuter", c.GtOuter(outer), Gt},
		{"GteOuter", c.GteOuter(outer), Gte},
		{"LtOuter", c.LtOuter(outer), Lt},
		{"LteOuter", c.LteOuter(outer), Lte},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.pred.Render()
			if n.Kind != NBinary || n.Op != tc.op {
				t.Fatalf("%s: node = %+v", tc.name, n)
			}

			if _, ok := n.Value.(outerRefCore); !ok {
				t.Fatalf("%s: Value type %T, want an outerRefCore marker", tc.name, n.Value)
			}
		})
	}
}

// TestColumnOuterCorrelatedRoundTrip proves correlation works end to end:
// the inner query selects only THIS widget's own orders, so w1 (two
// orders) and w3 (one order) match while w2's orders do not line up.
func TestColumnOuterCorrelatedRoundTrip(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgetOrders).
		Where(And(
			orderWidgetID.EqOuter(Outer(widgetID)),
			woAmount.Gt(20),
		)).
		Columns(orderWidgetID.Col())

	got, err := From(widgets).
		Where(widgetID.InSub(inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w2", "w3")
}

// TestNullableColumnNotInSub covers the NullableColumn NOT IN subquery
// surface: every widget whose bio is not w3's survives, while the NULL
// bio row is excluded by three-valued logic.
func TestNullableColumnNotInSub(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgets).Where(widgetID.Eq("w3")).Columns(widgetBio.Col())

	got, err := From(widgets).
		Where(widgetBio.NotInSub(inner)).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	wantIDs(t, got, "w1")
}

// TestNullableColumnScalarSub covers the NullableColumn scalar-comparison
// surface against a one-row inner query.
func TestNullableColumnScalarSub(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	// w3's bio is 'third'.
	inner := From(widgets).Where(widgetID.Eq("w3")).Columns(widgetBio.Col())

	got, err := From(widgets).Where(widgetBio.EqScalar(inner)).OrderBy(widgetID.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("EqScalar: %v", err)
	}

	wantIDs(t, got, "w3")

	for _, tc := range []struct {
		name string
		pred Predicate[widget]
	}{
		{"neq", widgetBio.NeqScalar(inner)},
		{"gt", widgetBio.GtScalar(inner)},
		{"gte", widgetBio.GteScalar(inner)},
		{"lt", widgetBio.LtScalar(inner)},
		{"lte", widgetBio.LteScalar(inner)},
	} {
		if !tc.pred.IsSet() {
			t.Fatalf("%s: scalar predicate unset", tc.name)
		}
	}
}

// TestColumnArrayNodeShape pins the erased NArray shape: the Op selects the
// comparison and quantifier, and Value is the bound element list.
func TestColumnArrayNodeShape(t *testing.T) {
	c := NewColumn[colT, int64]("t", "n")

	cases := []struct {
		name string
		pred Predicate[colT]
		op   Op
	}{
		{"EqAny", c.EqAny(1, 2), EqAny},
		{"NeqAny", c.NeqAny(1), NeqAny},
		{"EqAll", c.EqAll(1), EqAll},
		{"NeqAll", c.NeqAll(), NeqAll},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.pred.Render()
			if n.Kind != NArray || n.Op != tc.op {
				t.Fatalf("%s: node = %+v", tc.name, n)
			}

			if _, ok := n.Value.([]any); !ok {
				t.Fatalf("%s: Value type %T, want []any", tc.name, n.Value)
			}
		})
	}

	nc := NewNullableColumn[colT, int64]("t", "n")
	for _, tc := range []struct {
		name string
		pred Predicate[colT]
		op   Op
	}{
		{"EqAny", nc.EqAny(1), EqAny},
		{"NeqAny", nc.NeqAny(1), NeqAny},
		{"EqAll", nc.EqAll(1), EqAll},
		{"NeqAll", nc.NeqAll(1), NeqAll},
	} {
		t.Run("nullable/"+tc.name, func(t *testing.T) {
			n := tc.pred.Render()
			if n.Kind != NArray || n.Op != tc.op {
				t.Fatalf("%s: node = %+v", tc.name, n)
			}
		})
	}
}

// TestColumnArrayUnsupportedOnSQLite proves the array-quantifier surface is
// a typed dialect.ErrUnsupportedByDialect on SQLite (which has no
// ARRAY[...]/ANY/ALL) through the public All API.
func TestColumnArrayUnsupportedOnSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	_, err := From(widgets).Where(widgetQty.EqAny(10, 20)).All(ctx, conn)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("EqAny err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}

	_, err = From(widgets).Where(widgetQty.NeqAll(10)).All(ctx, conn)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("NeqAll err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}
