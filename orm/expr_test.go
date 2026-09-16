package orm

import (
	"context"
	"reflect"
	"testing"
)

// idsOf returns the ID field of every scanned widget, for order/result
// assertions.
func idsOf(rows []*widget) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}

	return out
}

func TestExprCoalesceSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	// bio is NULL for w2, so COALESCE(bio, 'unknown') equals 'unknown' only
	// there.
	rows, err := From(widgets).
		Where(CoalesceNullable(widgetBio, "unknown").Eq("unknown")).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if got, want := idsOf(rows), []string{"w2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("coalesce-matched ids = %v, want %v", got, want)
	}
}

// TestExprCoalesceBareForm covers the Column-seeded Coalesce: w2's NULL bio
// falls back, every other row keeps its bio.
func TestExprCoalesceBareForm(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rows, err := From(widgets).
		Where(Coalesce(widgetBio.Expr(), "zzz").Eq("zzz")).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if got, want := idsOf(rows), []string{"w2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("coalesce-matched ids = %v, want %v", got, want)
	}
}

func TestExprLowerUpperSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rows, err := From(widgets).Where(Lower(widgetName.Expr()).Eq("alpha")).All(ctx, conn)
	if err != nil {
		t.Fatalf("lower All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w1"}) {
		t.Fatalf("LOWER(name)='alpha' ids = %v, want [w1]", got)
	}

	rows, err = From(widgets).Where(Upper(widgetName.Expr()).Eq("GAMMA")).All(ctx, conn)
	if err != nil {
		t.Fatalf("upper All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w3"}) {
		t.Fatalf("UPPER(name)='GAMMA' ids = %v, want [w3]", got)
	}
}

// TestExprTrimNestedSQLite inserts a whitespace-padded row and matches it
// through nested LOWER(TRIM(...)), proving expressions nest.
func TestExprTrimNestedSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	if _, err := conn.Exec(ctx, `INSERT INTO widgets (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, "w4", "  Delta  ", int64(40), nil); err != nil {
		t.Fatalf("insert: %v", err)
	}

	rows, err := From(widgets).Where(Lower(Trim(widgetName.Expr())).Eq("delta")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w4"}) {
		t.Fatalf("LOWER(TRIM(name))='delta' ids = %v, want [w4]", got)
	}
}

func TestExprNullIfSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rows, err := From(widgets).
		Where(NullIf(widgetName.Expr(), "Alpha").IsNull()).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("is null All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w1"}) {
		t.Fatalf("NULLIF(name,'Alpha') IS NULL ids = %v, want [w1]", got)
	}

	rows, err = From(widgets).
		Where(NullIf(widgetName.Expr(), "Alpha").IsNotNull()).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("is not null All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w2", "w3"}) {
		t.Fatalf("NULLIF(name,'Alpha') IS NOT NULL ids = %v, want [w2 w3]", got)
	}
}

func TestExprLengthSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	// Alpha and Gamma are 5 characters, Beta is 4.
	rows, err := From(widgets).
		Where(Length(widgetName.Expr()).Eq(int64(5))).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w1", "w3"}) {
		t.Fatalf("LENGTH(name)=5 ids = %v, want [w1 w3]", got)
	}
}

// TestExprComparisonNodeShapes pins every Expr comparison rooting: each
// method tags the NFunc node with its Op and comparison value.
func TestExprComparisonNodeShapes(t *testing.T) {
	e := Lower(widgetName.Expr())

	cases := []struct {
		name string
		pred Predicate[widget]
		op   Op
	}{
		{"Eq", e.Eq("a"), Eq},
		{"Neq", e.Neq("a"), Neq},
		{"Gt", e.Gt("a"), Gt},
		{"Gte", e.Gte("a"), Gte},
		{"Lt", e.Lt("a"), Lt},
		{"Lte", e.Lte("a"), Lte},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := tc.pred.Render()
			if n.Kind != NFunc || n.Op != tc.op || n.Value != "a" {
				t.Fatalf("%s: node = %+v", tc.name, n)
			}
		})
	}

	in := e.In("a", "b")
	if n := in.Render(); n.Kind != NFunc || n.Op != In {
		t.Fatalf("In: node = %+v", n)
	}

	if n := e.Asc(); n.Func == nil || n.Desc {
		t.Fatalf("Asc: %+v", n)
	}

	if n := e.Desc(); n.Func == nil || !n.Desc {
		t.Fatalf("Desc: %+v", n)
	}
}

func TestExprCaseSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	expr := Case[widget, string]().When(widgetQty.Gte(20), "big").Else("small")

	rows, err := From(widgets).Where(expr.Eq("big")).OrderBy(widgetID.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("big All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w2", "w3"}) {
		t.Fatalf("CASE qty>=20 ids = %v, want [w2 w3]", got)
	}

	rows, err = From(widgets).Where(expr.Eq("small")).OrderBy(widgetID.Asc()).All(ctx, conn)
	if err != nil {
		t.Fatalf("small All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w1"}) {
		t.Fatalf("CASE else ids = %v, want [w1]", got)
	}
}

func TestExprCaseMultiWhenSQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	expr := Case[widget, string]().
		When(widgetQty.Gte(30), "large").
		When(widgetQty.Gte(20), "medium").
		Else("small")

	rows, err := From(widgets).Where(expr.Eq("medium")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w2"}) {
		t.Fatalf("CASE multi-when ids = %v, want [w2]", got)
	}
}

// TestExprOrderBySQLite orders by COALESCE(bio, 'zzz'): the NULL row w2
// sorts last (after "first" and "third").
func TestExprOrderBySQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rows, err := From(widgets).
		OrderBy(CoalesceNullable(widgetBio, "zzz").Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if got, want := idsOf(rows), []string{"w1", "w3", "w2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered ids = %v, want %v", got, want)
	}
}

func TestExprComposesInAndOrNot(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rows, err := From(widgets).
		Where(And(CoalesceNullable(widgetBio, "x").Eq("x"), Lower(widgetName.Expr()).Eq("beta"))).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("and All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w2"}) {
		t.Fatalf("AND ids = %v, want [w2]", got)
	}

	rows, err = From(widgets).
		Where(Or(widgetQty.Eq(10), Lower(widgetName.Expr()).Eq("gamma"))).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("or All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w1", "w3"}) {
		t.Fatalf("OR ids = %v, want [w1 w3]", got)
	}

	rows, err = From(widgets).
		Where(Not(CoalesceNullable(widgetBio, "x").Eq("first"))).
		OrderBy(widgetID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("not All: %v", err)
	}

	if got := idsOf(rows); !reflect.DeepEqual(got, []string{"w2", "w3"}) {
		t.Fatalf("NOT ids = %v, want [w2 w3]", got)
	}
}

// TestExprBoundFallbackPlaceholderOrder proves an expression's bound
// fallback argument is numbered BEFORE the outer comparison value, matching
// the SQL text order -- both on the fluent Postgres path and against real
// SQLite.
func TestExprBoundFallbackPlaceholderOrder(t *testing.T) {
	ctx := context.Background()

	rec := &recordingExec{dialectName: "postgres"}

	_, _ = From(widgets).
		Where(And(widgetQty.Gt(10), CoalesceNullable(widgetBio, "x").Eq("x"))).
		All(ctx, rec)

	q, args := rec.last()
	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE ("quantity" > $1 AND COALESCE("bio", $2) = $3)`
	if q != want {
		t.Fatalf("postgres query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{int64(10), "x", "x"}) {
		t.Fatalf("postgres args = %#v, want [10 x x]", args)
	}

	sqRec := &recordingExec{dialectName: "sqlite"}

	_, _ = From(widgets).
		Where(And(widgetQty.Gt(10), CoalesceNullable(widgetBio, "x").Eq("x"))).
		All(ctx, sqRec)

	sqQ, sqArgs := sqRec.last()
	if !reflect.DeepEqual(sqArgs, []any{int64(10), "x", "x"}) {
		t.Fatalf("sqlite query = %q args = %#v, want [10 x x]", sqQ, sqArgs)
	}
}

func TestExprOrderByPostgresRender(t *testing.T) {
	ctx := context.Background()

	rec := &recordingExec{dialectName: "postgres"}

	_, _ = From(widgets).OrderBy(CoalesceNullable(widgetBio, "z").Desc()).All(ctx, rec)

	q, args := rec.last()
	want := `SELECT "id", "name", "quantity", "bio" FROM "widgets" ORDER BY COALESCE("bio", $1) DESC`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"z"}) {
		t.Fatalf("args = %#v, want [z]", args)
	}
}
