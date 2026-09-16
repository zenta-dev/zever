package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

func colLeaf(col string) Node                 { return Node{Kind: KindColumn, Column: col} }
func litLeaf(v any) Node                      { return Node{Kind: KindLit, Value: v} }
func fnNode(name string, a ...Node) *FuncExpr { return &FuncExpr{Name: name, Args: a} }

func TestSelectFuncPredicatePostgres(t *testing.T) {
	t.Parallel()
	where := Node{
		Kind:  KindFunc,
		Func:  fnNode("COALESCE", colLeaf("bio"), litLeaf("n/a")),
		Op:    OpEq,
		Value: "n/a",
	}

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "widgets" WHERE COALESCE("bio", $1) = $2`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"n/a", "n/a"}) {
		t.Fatalf("args = %#v, want [n/a n/a]", args)
	}
}

func TestSelectFuncPredicateSQLite(t *testing.T) {
	t.Parallel()
	where := Node{
		Kind:  KindFunc,
		Func:  fnNode("COALESCE", colLeaf("bio"), litLeaf("n/a")),
		Op:    OpEq,
		Value: "n/a",
	}

	q, args, err := Select(sqlite.New(), "widgets", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "widgets" WHERE COALESCE("bio", ?) = ?`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"n/a", "n/a"}) {
		t.Fatalf("args = %#v, want [n/a n/a]", args)
	}
}

func TestSelectFuncNested(t *testing.T) {
	t.Parallel()
	inner := Node{Kind: KindFunc, Func: fnNode("COALESCE", colLeaf("name"), litLeaf("d"))}
	where := Node{Kind: KindFunc, Func: fnNode("LOWER", inner), Op: OpEq, Value: "x"}

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "widgets" WHERE LOWER(COALESCE("name", $1)) = $2`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"d", "x"}) {
		t.Fatalf("args = %#v, want [d x]", args)
	}
}

// TestSelectFuncInWhereAndOrderIdentical proves the same expression tree
// renders byte-identically as a WHERE operand and as an ORDER BY term.
func TestSelectFuncInWhereAndOrderIdentical(t *testing.T) {
	t.Parallel()
	fn := func() *FuncExpr { return fnNode("COALESCE", colLeaf("bio"), litLeaf("z")) }

	where := Node{Kind: KindFunc, Func: fn(), Op: OpEq, Value: "z"}
	order := []OrderTerm{{Func: fn()}}

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id"}, where, order, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "widgets" WHERE COALESCE("bio", $1) = $2 ORDER BY COALESCE("bio", $3) ASC`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"z", "z", "z"}) {
		t.Fatalf("args = %#v, want [z z z]", args)
	}
}

func TestSelectFuncOrderOnly(t *testing.T) {
	t.Parallel()
	order := []OrderTerm{{Func: fnNode("COALESCE", colLeaf("bio"), litLeaf("zzz")), Desc: true}}

	q, args, err := Select(sqlite.New(), "widgets", []string{"id"}, Node{}, order, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "widgets" ORDER BY COALESCE("bio", ?) DESC`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"zzz"}) {
		t.Fatalf("args = %#v, want [zzz]", args)
	}
}

func TestSelectFuncCase(t *testing.T) {
	t.Parallel()
	elseNode := litLeaf("small")
	caseExpr := &FuncExpr{
		Case: true,
		Whens: []FuncWhen{{
			Cond: Node{Kind: KindBinary, Column: "quantity", Op: OpGte, Value: 20},
			Then: litLeaf("big"),
		}},
		Else: &elseNode,
	}

	where := Node{Kind: KindFunc, Func: caseExpr, Op: OpEq, Value: "big"}

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "widgets" WHERE CASE WHEN "quantity" >= $1 THEN $2 ELSE $3 END = $4`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{20, "big", "small", "big"}) {
		t.Fatalf("args = %#v, want [20 big small big]", args)
	}
}

func TestSelectFuncCaseWithoutWhensFailsClosed(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindFunc, Func: &FuncExpr{Case: true}, Op: OpEq, Value: "x"}

	_, _, err := Select(sqlite.New(), "widgets", []string{"id"}, where, nil, 0, 0)
	if err == nil {
		t.Fatalf("err = nil, want a CASE-without-WHEN error")
	}

	if !strings.Contains(err.Error(), "CASE expression requires at least one WHEN arm") {
		t.Fatalf("err = %v, want it to mention the missing WHEN arm", err)
	}
}

// TestSelectFuncLengthDialect proves LENGTH renders as LENGTH on both
// supported dialects.
func TestSelectFuncLengthDialect(t *testing.T) {
	t.Parallel()
	where := Node{Kind: KindFunc, Func: fnNode("LENGTH", colLeaf("name")), Op: OpEq, Value: int64(5)}

	pg, _, err := Select(fakePostgres{}, "widgets", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("postgres err = %v, want nil", err)
	}

	if !strings.Contains(pg, `LENGTH("name") = $1`) {
		t.Fatalf("postgres query = %q, want LENGTH(\"name\")", pg)
	}

	lite, _, err := Select(sqlite.New(), "widgets", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("sqlite err = %v, want nil", err)
	}

	if !strings.Contains(lite, `LENGTH("name") = ?`) {
		t.Fatalf("sqlite query = %q, want LENGTH(\"name\")", lite)
	}
}

func TestSelectFuncPlaceholderOrderWithOuterArg(t *testing.T) {
	t.Parallel()
	inner := Node{Kind: KindFunc, Func: fnNode("COALESCE", colLeaf("bio"), litLeaf("x")), Op: OpEq, Value: "x"}
	outer := Node{Kind: KindBinary, Column: "quantity", Op: OpGt, Value: 10}
	where := Node{Kind: KindCompound, Compound: CompoundAnd, Children: []Node{outer, inner}}

	q, args, err := Select(fakePostgres{}, "widgets", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "widgets" WHERE ("quantity" > $1 AND COALESCE("bio", $2) = $3)`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{10, "x", "x"}) {
		t.Fatalf("args = %#v, want [10 x x]", args)
	}
}

func TestSelectFuncInAndIsNull(t *testing.T) {
	t.Parallel()
	in := Node{Kind: KindFunc, Func: fnNode("COALESCE", colLeaf("bio"), litLeaf("f")), Op: OpIn, Value: []any{"a", "b"}}

	q, args, err := Select(sqlite.New(), "widgets", []string{"id"}, in, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "widgets" WHERE COALESCE("bio", ?) IN (?, ?)`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"f", "a", "b"}) {
		t.Fatalf("args = %#v, want [f a b]", args)
	}

	isNull := Node{Kind: KindFunc, Func: fnNode("COALESCE", colLeaf("bio"), litLeaf("f")), Op: OpIsNull}

	q, args, err = Select(sqlite.New(), "widgets", []string{"id"}, isNull, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want = `SELECT "id" FROM "widgets" WHERE COALESCE("bio", ?) IS NULL`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"f"}) {
		t.Fatalf("args = %#v, want [f]", args)
	}
}
