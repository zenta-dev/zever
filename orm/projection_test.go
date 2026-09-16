package orm

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestProjectedQuerySQLite drives expression projection end to end: three
// aliased scalar expressions (COALESCE/LOWER/CASE) scanned positionally into
// a caller-defined destination, proving result rows no longer correspond to
// T and are scanned through ProjectedRow.Scan.
func TestProjectedQuerySQLite(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	type row struct{ Bio, LName, Size string }

	var got []row

	err := From(widgets).OrderBy(widgetID.Asc()).Project(
		CoalesceNullable(widgetBio, "").As("bio"),
		Lower(widgetName.Expr()).As("lname"),
		Case[widget, string]().When(widgetQty.Gte(20), "big").Else("small").As("size"),
	).All(ctx, conn, func(r ProjectedRow) error {
		var x row
		if err := r.Scan(&x.Bio, &x.LName, &x.Size); err != nil {
			return err
		}

		got = append(got, x)

		return nil
	})
	if err != nil {
		t.Fatalf("ProjectedQuery.All: %v", err)
	}

	want := []row{
		{"first", "alpha", "small"},
		{"", "beta", "big"},
		{"third", "gamma", "big"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projected rows = %#v, want %#v", got, want)
	}
}

// TestProjectedQueryFirst proves First scans one projected row into the
// caller's destinations and reports ok=false when nothing matches.
func TestProjectedQueryFirst(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var bio, lname string

	ok, err := From(widgets).Where(widgetID.Eq("w2")).Project(
		CoalesceNullable(widgetBio, "").As("bio"),
		Lower(widgetName.Expr()).As("lname"),
	).First(ctx, conn, &bio, &lname)
	if err != nil || !ok {
		t.Fatalf("First: ok=%v err=%v", ok, err)
	}

	if bio != "" || lname != "beta" {
		t.Fatalf("First scanned (%q, %q), want (\"\", \"beta\")", bio, lname)
	}

	ok, err = From(widgets).Where(widgetID.Eq("missing")).Project(
		CoalesceNullable(widgetBio, "").As("bio"),
	).First(ctx, conn, &bio)
	if err != nil {
		t.Fatalf("First(missing): %v", err)
	}

	if ok {
		t.Fatalf("First(missing) ok = true, want false")
	}
}

// TestProjectedQueryColumnAndNullableAlias proves Column.As / NullableColumn.As
// project plain columns under aliases, and carry their WHERE/ORDER/LIMIT
// modifiers through the projected query.
func TestProjectedQueryColumnAndNullableAlias(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	type row struct {
		Wid string
		Bio string
	}

	var got []row

	err := From(widgets).
		Where(widgetQty.Gte(20)).
		OrderBy(widgetID.Desc()).
		Limit(1).
		Project(
			widgetID.As("wid"),
			widgetBio.As("bio"),
		).All(ctx, conn, func(r ProjectedRow) error {
		var x row
		if err := r.Scan(&x.Wid, &x.Bio); err != nil {
			return err
		}

		got = append(got, x)

		return nil
	})
	if err != nil {
		t.Fatalf("ProjectedQuery.All: %v", err)
	}

	if !reflect.DeepEqual(got, []row{{"w3", "third"}}) {
		t.Fatalf("rows = %#v, want [{w3 third}]", got)
	}
}

// TestProjectedQueryScalarSubquery proves a (correlated) scalar subquery can
// be projected as an aliased output column: each widget's id is echoed back
// by a correlated inner SELECT, NULL when the subquery finds no row.
func TestProjectedQueryScalarSubquery(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	inner := From(widgetOrders).
		Columns(orderWidgetID.Col()).
		Where(orderWidgetID.EqOuter(Outer(widgetID))).
		OrderBy(orderID.Asc()).
		Limit(1)

	type row struct {
		ID    string
		Match Option[string]
	}

	var got []row

	err := From(widgets).OrderBy(widgetID.Asc()).Project(
		widgetID.As("id"),
		ScalarSubquery[widget, string](inner).As("match"),
	).All(ctx, conn, func(r ProjectedRow) error {
		var x row
		if err := r.Scan(&x.ID, &x.Match); err != nil {
			return err
		}

		got = append(got, x)

		return nil
	})
	if err != nil {
		t.Fatalf("ProjectedQuery.All: %v", err)
	}

	if len(got) != 3 || got[0].ID != "w1" || got[1].ID != "w2" || got[2].ID != "w3" {
		t.Fatalf("ids = %#v, want w1,w2,w3", got)
	}

	for _, x := range got {
		if v, ok := x.Match.Get(); !ok || v != x.ID {
			t.Fatalf("widget %s correlated match = %#v, want some(%s)", x.ID, x.Match, x.ID)
		}
	}
}

// TestProjectedQueryRendersAliases captures the emitted SQL and asserts the
// `AS "alias"` clauses and DISTINCT prefix are present.
func TestProjectedQueryRendersAliases(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rec := &ormCaptureDB{DB: conn}

	err := From(widgets).Distinct().Project(
		CoalesceNullable(widgetBio, "").As("bio"),
		Lower(widgetName.Expr()).As("lname"),
	).All(ctx, rec, func(ProjectedRow) error { return nil })
	if err != nil {
		t.Fatalf("ProjectedQuery.All: %v", err)
	}

	q, args := rec.lastQuery()

	for _, frag := range []string{"SELECT DISTINCT", `COALESCE("bio", `, `AS "bio"`, `LOWER("name")`, `AS "lname"`} {
		if !strings.Contains(q, frag) {
			t.Fatalf("query %q missing %q", q, frag)
		}
	}

	if !reflect.DeepEqual(args, []any{""}) {
		t.Fatalf("args = %#v, want [\"\"]", args)
	}
}

// TestProjectedQueryEmptyErrors proves a projected query with no projections
// is a typed ErrProjectionEmpty rather than an invalid `SELECT FROM` statement.
func TestProjectedQueryEmptyErrors(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	err := From(widgets).Project().All(ctx, conn, func(ProjectedRow) error { return nil })
	if !errors.Is(err, ErrProjectionEmpty) {
		t.Fatalf("All err = %v, want errors.Is(err, ErrProjectionEmpty)", err)
	}

	if _, err := From(widgets).Project().First(ctx, conn); !errors.Is(err, ErrProjectionEmpty) {
		t.Fatalf("First err = %v, want errors.Is(err, ErrProjectionEmpty)", err)
	}
}

// TestProjectedQueryEmptyAlias proves an empty alias renders the bare
// expression (no `AS`), which is still scannable positionally.
func TestProjectedQueryEmptyAlias(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	rec := &ormCaptureDB{DB: conn}

	err := From(widgets).OrderBy(widgetID.Asc()).Limit(1).Project(
		Lower(widgetName.Expr()).As(""),
	).All(ctx, rec, func(ProjectedRow) error { return nil })
	if err != nil {
		t.Fatalf("ProjectedQuery.All: %v", err)
	}

	q, _ := rec.lastQuery()

	if strings.Contains(q, " AS ") {
		t.Fatalf("query %q rendered AS for an empty alias", q)
	}
}

// TestProjectedQueryBranchSafe proves ProjectedQuery's modifiers are
// copy-on-write: branching a base projected query leaves the base (and its
// sibling) untouched.
func TestProjectedQueryBranchSafe(t *testing.T) {
	base := From(widgets).Project(widgetID.As("id"))

	a := base.Where(widgetID.Eq("w1"))
	b := base.Limit(1)

	if a.limit != 0 {
		t.Fatalf("a.limit = %d, want 0 (base unmodified)", a.limit)
	}

	if b.where.IsSet() {
		t.Fatalf("b.where set after branching, want unset")
	}
}

// TestProjectedQueryCorrelatedScopeError proves a projected scalar subquery
// whose marker does not match the projected query's own table fails closed
// with a rendering error rather than emitting silently-wrong SQL.
func TestProjectedQueryCorrelatedScopeError(t *testing.T) {
	ctx, conn := newWidgetOrdersDB(t)

	// The projected query's table is widget_orders, but the marker references
	// widgets.id -- not one of the enclosing statement's tables.
	bad := From(widgetOrders).
		Columns(orderWidgetID.Col()).
		Where(orderWidgetID.EqOuter(Outer(widgetID)))

	err := From(widgetOrders).Project(
		ScalarSubquery[widgetOrder, string](bad).As("x"),
	).All(ctx, conn, func(ProjectedRow) error { return nil })
	if err == nil {
		t.Fatalf("err = nil, want a rendering-time correlated-scope error")
	}
}

// TestProjectedQueryChainAndErrorPaths proves ProjectedQuery's Where/
// OrderBy/Offset/Distinct chain composes and every execution error path
// fails closed.
func TestProjectedQueryChainAndErrorPaths(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	var got []string

	err := From(widgets).
		Where(widgetQty.Gt(int64(5))).
		Project(widgetID.As("id")).
		Where(widgetName.Neq("Gamma")).
		OrderBy(widgetID.Asc()).
		Distinct().
		Offset(0).
		All(ctx, conn, func(row ProjectedRow) error {
			var id string

			if err := row.Scan(&id); err != nil {
				return err
			}

			got = append(got, id)

			return nil
		})
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 2 || got[0] != "w1" || got[1] != "w2" {
		t.Fatalf("ids = %v, want [w1 w2]", got)
	}

	p := From(widgets).Project(widgetID.As("id"))

	err = p.All(ctx, fakeDB{}, func(ProjectedRow) error { return nil })
	if err == nil {
		t.Fatal("All on an unresolvable dialect succeeded, want an error")
	}

	boom := errors.New("boom")
	stub := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, queryErr: boom}

	err = p.All(ctx, stub, func(ProjectedRow) error { return nil })
	if !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	fnErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{values: [][]any{{"w1"}}}}

	err = p.All(ctx, fnErr, func(ProjectedRow) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	iterErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{values: [][]any{{"w1"}}, iterErr: boom}}

	err = p.All(ctx, iterErr, func(ProjectedRow) error { return nil })
	if !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	closeErr := &ormStubDB{mockExec: mockExec{dialectName: "sqlite"}, rows: &stubRows{closeErr: boom}}

	err = p.All(ctx, closeErr, func(ProjectedRow) error { return nil })
	if !errors.Is(err, boom) {
		t.Fatalf("All err = %v, want errors.Is(err, boom)", err)
	}

	ok, err := From(widgets).Where(widgetQty.Gt(int64(1000))).Project(widgetID.As("id")).First(ctx, conn)
	if err != nil {
		t.Fatalf("First: %v", err)
	}

	if ok {
		t.Fatal("First = true, want false (no rows)")
	}

	if _, err := p.First(ctx, fakeDB{}); err == nil {
		t.Fatal("First on an unresolvable dialect succeeded, want an error")
	}
}

// TestProjectedQueryValidateAndFirstScanEdges proves the locking gate
// rejects a locked projected query and First surfaces a row-scan failure.
func TestProjectedQueryValidateAndFirstScanEdges(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	locked := From(widgets).ForUpdate().Project(widgetID.As("id"))

	if err := locked.All(ctx, mockExec{dialectName: "sqlite"}, func(ProjectedRow) error { return nil }); err == nil {
		t.Fatal("All with FOR UPDATE on sqlite succeeded, want a locking error")
	}

	var dst struct{}

	if _, err := From(widgets).Project(widgetID.As("id")).First(ctx, conn, &dst); err == nil {
		t.Fatal("First scanning into *struct succeeded, want a scan error")
	}
}
