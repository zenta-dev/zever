package render

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	postgresd "github.com/zenta-dev/zever/orm/dialect/postgres"
	sqlited "github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestOrderByNullsPostgresExactSQL pins the exact Postgres SQL for both
// NULLS modifiers: the pre-existing ASC/DESC token always stays, and the
// NULLS suffix follows it.
func TestOrderByNullsPostgresExactSQL(t *testing.T) {
	resetShapeCache()

	q, args, err := Select(postgresd.New(), "widgets", []string{"id", "name"}, Node{},
		[]OrderTerm{
			{Column: "bio", Nulls: NullsFirst},
			{Column: "quantity", Desc: true, Nulls: NullsLast},
		}, 0, 0)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	want := `SELECT "id", "name" FROM "widgets" ORDER BY "bio" ASC NULLS FIRST, "quantity" DESC NULLS LAST`
	if q != want {
		t.Fatalf("SQL:\n got: %q\nwant: %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none (NULLS modifiers bind nothing)", args)
	}
}

// TestOrderByNullsPlaceholderNumberingPostgres proves the suffix never
// shifts the $N numbering of a scalar-expression term's bound args.
func TestOrderByNullsPlaceholderNumberingPostgres(t *testing.T) {
	resetShapeCache()

	fn := &FuncExpr{Name: "COALESCE", Args: []Node{
		{Kind: KindColumn, Column: "bio"},
		{Kind: KindLit, Value: "zzz"},
	}}

	q, args, err := Select(postgresd.New(), "widgets", []string{"id"},
		nBinary("widgets", "quantity", OpGt, int64(5)),
		[]OrderTerm{{Func: fn, Nulls: NullsLast}}, 0, 0)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	want := `SELECT "id" FROM "widgets" WHERE "quantity" > $1 ORDER BY COALESCE("bio", $2) ASC NULLS LAST`
	if q != want {
		t.Fatalf("SQL:\n got: %q\nwant: %q", q, want)
	}

	if len(args) != 2 || args[0] != int64(5) || args[1] != "zzz" {
		t.Fatalf("args = %#v, want [5 zzz]", args)
	}
}

// TestOrderByNullsUnsupportedDialectTypedError pins that a dialect without
// NULLS FIRST/LAST support rejects the modifier with the typed
// dialect.ErrUnsupportedByDialect rather than silently dropping it.
func TestOrderByNullsUnsupportedDialectTypedError(t *testing.T) {
	resetShapeCache()

	_, _, err := Select(baseOnlyDialect{}, "widgets", []string{"id"}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, 0, 0)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

// TestOrderByNullsSQLiteVersionGate pins the SQLite >= 3.30.0 floor: the
// version-aware dialect rejects the modifier below it and renders the
// suffix at or above it. New() defaults to the modernc driver's bundled
// version (well above the floor).
func TestOrderByNullsSQLiteVersionGate(t *testing.T) {
	resetShapeCache()

	old, err := sqlited.NewWithVersion("3.29.5")
	if err != nil {
		t.Fatalf("NewWithVersion: %v", err)
	}

	if old.SupportsNullsOrdering() {
		t.Fatal("sqlite 3.29.5 SupportsNullsOrdering() = true, want false")
	}

	_, _, selErr := Select(old, "widgets", []string{"id"}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, 0, 0)
	if !errors.Is(selErr, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("sqlite 3.29.5 err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", selErr)
	}

	newer, err := sqlited.NewWithVersion("3.30.0")
	if err != nil {
		t.Fatalf("NewWithVersion: %v", err)
	}

	if !newer.SupportsNullsOrdering() {
		t.Fatal("sqlite 3.30.0 SupportsNullsOrdering() = false, want true")
	}

	q, _, err := Select(newer, "widgets", []string{"id"}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, 0, 0)
	if err != nil {
		t.Fatalf("sqlite 3.30.0 Select: %v", err)
	}

	want := `SELECT "id" FROM "widgets" ORDER BY "bio" ASC NULLS FIRST`
	if q != want {
		t.Fatalf("SQL:\n got: %q\nwant: %q", q, want)
	}

	if !sqlited.New().SupportsNullsOrdering() {
		t.Fatal("sqlite.New() SupportsNullsOrdering() = false, want true (modernc bundles a modern SQLite)")
	}
}

// TestOrderByNullsShapeCacheCapabilityIsolation is the cache-safety test:
// a supported SQLite version's cached NULLS text must never be served to an
// unsupported version of the same-named dialect -- the unsupported call
// must still fail with the typed error.
func TestOrderByNullsShapeCacheCapabilityIsolation(t *testing.T) {
	resetShapeCache()

	supported, err := sqlited.NewWithVersion("3.30.0")
	if err != nil {
		t.Fatalf("NewWithVersion: %v", err)
	}

	unsupported, err := sqlited.NewWithVersion("3.29.5")
	if err != nil {
		t.Fatalf("NewWithVersion: %v", err)
	}

	term := []OrderTerm{{Column: "bio", Nulls: NullsFirst}}

	if _, _, err := Select(supported, "widgets", []string{"id"}, Node{}, term, 0, 0); err != nil {
		t.Fatalf("supported Select: %v", err)
	}

	if _, _, err := Select(unsupported, "widgets", []string{"id"}, Node{}, term, 0, 0); !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("unsupported err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect) (cached supported text leaked)", err)
	}
}

// TestOrderByNullsShapeCacheHit proves a NULLS-bearing shape still renders
// byte-identically on the cache-hit path.
func TestOrderByNullsShapeCacheHit(t *testing.T) {
	renderTwice(t, func() (string, []any, error) {
		return Select(postgresd.New(), "widgets", []string{"id"}, Node{},
			[]OrderTerm{{Column: "bio", Nulls: NullsLast}}, 0, 0)
	})
}

// TestOrderByNullsModifiersDistinctShapeKeys proves the three NULLS
// positions fingerprint to three distinct cache entries -- a NULLS LAST
// shape can never hit a NULLS FIRST (or default) entry.
func TestOrderByNullsModifiersDistinctShapeKeys(t *testing.T) {
	resetShapeCache()

	d := postgresd.New()

	render := func(n NullsOrder) string {
		t.Helper()

		q, _, err := Select(d, "widgets", []string{"id"}, Node{}, []OrderTerm{{Column: "bio", Nulls: n}}, 0, 0)
		if err != nil {
			t.Fatalf("Select(%v): %v", n, err)
		}

		return q
	}

	def := render(NullsDefault)
	first := render(NullsFirst)
	last := render(NullsLast)

	if def == first || def == last || first == last {
		t.Fatalf("NULLS shapes collided:\n default=%q\n first=%q\n last=%q", def, first, last)
	}

	if n := shapeCacheLen(); n != 3 {
		t.Fatalf("shapeCacheLen() = %d, want 3 distinct NULLS-position shapes", n)
	}
}

// TestOrderByNullsCaseEmulation proves the portable NULLS-ordering
// workaround -- ORDER BY CASE WHEN col IS NULL THEN 1 ELSE 0 END, col --
// renders without tripping the NULLS gate (it carries no NULLS keyword).
func TestOrderByNullsCaseEmulation(t *testing.T) {
	resetShapeCache()

	elseNode := Node{Kind: KindLit, Value: 0}
	fn := &FuncExpr{
		Case:  true,
		Whens: []FuncWhen{{Cond: nBinary("", "bio", OpIsNull, nil), Then: Node{Kind: KindLit, Value: 1}}},
		Else:  &elseNode,
	}

	q, args, err := Select(sqlited.New(), "widgets", []string{"id"}, Node{},
		[]OrderTerm{{Func: fn}, {Column: "bio"}}, 0, 0)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}

	want := `SELECT "id" FROM "widgets" ORDER BY CASE WHEN "bio" IS NULL THEN ? ELSE ? END ASC, "bio" ASC`
	if q != want {
		t.Fatalf("SQL:\n got: %q\nwant: %q", q, want)
	}

	if len(args) != 2 || args[0] != 1 || args[1] != 0 {
		t.Fatalf("args = %#v, want [1 0]", args)
	}
}

// TestOrderByNullsThreadedThroughEveryRenderPath drives one NULLS term
// through every kind of ORDER BY render site -- plain SELECT, Join2,
// Join3, CTE, setop, window, UPDATE and DELETE -- and asserts the suffix is
// present in each. This is the guard against a render site copying the
// shared OrderTerm without carrying the Nulls field.
func TestOrderByNullsThreadedThroughEveryRenderPath(t *testing.T) {
	d := postgresd.New()

	// Plain SELECT.
	q, _, err := Select(d, "widgets", []string{"id"}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, 0, 0)
	requireNullsFirst(t, "Select", q, err)

	// Join2 (LEFT table term).
	q, _, err = SelectJoin(d, InnerJoin,
		"left_t", []string{"id"}, "right_t", []string{"id"}, "id", "left_id",
		Node{}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, nil, 0, 0)
	requireNullsFirst(t, "SelectJoin", q, err)

	// Join2 (RIGHT table term -- exercises qualifyOrderTerms).
	q, _, err = SelectJoin(d, InnerJoin,
		"left_t", []string{"id"}, "right_t", []string{"id"}, "id", "left_id",
		Node{}, Node{},
		nil, []OrderTerm{{Column: "note", Nulls: NullsFirst}}, 0, 0)
	requireNullsFirst(t, "SelectJoin/right", q, err)

	// Join3.
	q, _, err = SelectJoin3(d, InnerJoin,
		"a_t", []string{"id"}, "b_t", []string{"id"}, "c_t", []string{"id"},
		"id", "a_id", "id", "b_id",
		Node{}, Node{}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, nil, nil, 0, 0)
	requireNullsFirst(t, "SelectJoin3", q, err)

	// CTE outer ORDER BY.
	q, _, err = SelectWith(d, "w", false,
		CTEBody{Kind: CTEBodyPlain, Table: "widgets", Columns: []string{"id", "bio"}},
		[]string{"id", "bio"}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, 0, 0)
	requireNullsFirst(t, "SelectWith", q, err)

	// Set operation result ORDER BY.
	q, _, err = SetOp(d, SetOpUnion, "a", []string{"id"}, Node{}, "b", []string{"id"}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, 0, 0)
	requireNullsFirst(t, "SetOp", q, err)

	// Window SELECT result ORDER BY.
	q, _, err = WindowSelect(d, "widgets", []string{"id"}, nil, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, 0, 0)
	requireNullsFirst(t, "WindowSelect", q, err)

	// Window OVER (ORDER BY ...) term.
	q, _, err = WindowSelect(d, "widgets", []string{"id"},
		[]WindowExpr{{Func: WinRowNumber, Alias: "rn", Over: OverClause{Order: []OrderTerm{{Column: "bio", Nulls: NullsFirst}}}}},
		Node{}, nil, 0, 0)
	requireNullsFirst(t, "WindowSelect/over", q, err)

	// UPDATE tail.
	q, _, err = Update(d, "widgets", []Assignment{{Column: "name", Value: "x"}}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, 0, 0)
	requireNullsFirst(t, "Update", q, err)

	// DELETE tail.
	q, _, err = Delete(d, "widgets", Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsFirst}}, 0, 0)
	requireNullsFirst(t, "Delete", q, err)
}

func requireNullsFirst(t *testing.T, label, q string, err error) {
	t.Helper()

	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}

	if !strings.Contains(q, "NULLS FIRST") {
		t.Fatalf("%s SQL %q drops the NULLS FIRST modifier", label, q)
	}
}
