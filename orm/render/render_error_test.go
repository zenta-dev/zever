package render

import (
	"reflect"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestRenderExprUnsupportedNodeKind proves an unsupported/unknown NodeKind is
// an explicit rendering error rather than a silently-dropped clause.
// KindFunc is no longer in this set: it is now a supported kind (the scalar
// expression algebra, see renderFuncPredicate), and its own malformed-payload
// failure is covered by TestRenderFuncMalformedPayload.
func TestRenderExprUnsupportedNodeKind(t *testing.T) {
	t.Parallel()
	for _, kind := range []NodeKind{KindLit, KindColumn, KindUnary, NodeKind(99)} {
		t.Run(kindName(kind), func(t *testing.T) {
			t.Parallel()
			_, _, err := renderExpr(sqlite.New(), Node{Kind: kind}, &argCounter{})
			if err == nil {
				t.Fatalf("renderExpr(kind=%d) err = nil, want unsupported node kind error", kind)
			}

			if !strings.Contains(err.Error(), "unsupported node kind") {
				t.Fatalf("err = %v, want it to mention unsupported node kind", err)
			}
		})
	}
}

// TestRenderFuncMalformedPayload proves a KindFunc node with no Func payload
// (impossible through orm/expr.go, but a malformed caller-built node) is an
// explicit error rather than a silently empty clause.
func TestRenderFuncMalformedPayload(t *testing.T) {
	t.Parallel()
	_, _, err := renderExpr(sqlite.New(), Node{Kind: KindFunc, Op: OpEq, Value: "x"}, &argCounter{})
	if err == nil {
		t.Fatalf("renderExpr(func without payload) err = nil, want an error")
	}
}

// TestSelectPropagatesUnsupportedNodeKind proves the error propagates through
// a top-level render entry point (Select), not just renderExpr.
func TestSelectPropagatesUnsupportedNodeKind(t *testing.T) {
	t.Parallel()
	_, _, err := Select(sqlite.New(), "widgets", []string{"id"}, Node{Kind: KindUnary}, nil, 0, 0)
	if err == nil {
		t.Fatalf("Select err = nil, want unsupported node kind error")
	}
}

// TestRenderBinaryUnknownOperator proves an out-of-range Op is an explicit
// error rather than silently rendering `=`.
func TestRenderBinaryUnknownOperator(t *testing.T) {
	t.Parallel()
	_, _, err := renderExpr(sqlite.New(), Node{Kind: KindBinary, Column: "id", Op: Op(99), Value: "x"}, &argCounter{})
	if err == nil {
		t.Fatalf("renderExpr(Op=99) err = nil, want unknown operator error")
	}

	if !strings.Contains(err.Error(), "unknown operator") {
		t.Fatalf("err = %v, want it to mention unknown operator", err)
	}
}

// TestRenderRawMalformedPayload proves a KindRaw node whose Value is not a
// RawExpr is an explicit error rather than a silently empty clause.
func TestRenderRawMalformedPayload(t *testing.T) {
	t.Parallel()
	_, _, err := renderExpr(sqlite.New(), Node{Kind: KindRaw, Value: "not-a-raw-expr"}, &argCounter{})
	if err == nil {
		t.Fatalf("renderExpr(malformed raw) err = nil, want an error")
	}
}

func kindName(k NodeKind) string {
	switch k { //nolint:exhaustive // name only the kinds that reach here; others fall to the generic name
	case KindLit:
		return "lit"
	case KindColumn:
		return "column"
	case KindUnary:
		return "unary"
	case KindFunc:
		return "func"
	default:
		return "unknown"
	}
}

func TestRenderBinaryNotABinaryOperator(t *testing.T) {
	t.Parallel()

	_, _, err := renderExpr(sqlite.New(), Node{Kind: KindBinary, Column: "id", Op: OpIn, Value: "x"}, &argCounter{})
	if err == nil {
		t.Fatal("err = nil, want a not-a-binary-comparison error")
	}

	if !strings.Contains(err.Error(), "is not a binary comparison") {
		t.Fatalf("err = %v, want it to mention binary comparison", err)
	}
}

func TestRenderScalarSubqueryInnerError(t *testing.T) {
	t.Parallel()

	bad := nSub("widget_orders", []string{"widget_id"},
		Node{Kind: KindBinary, Column: "amount", Op: OpEqAny, Value: 1})

	_, _, err := renderExpr(sqlite.New(),
		Node{Kind: KindBinary, Column: "id", Op: OpEq, Value: bad}, &argCounter{})
	if err == nil {
		t.Fatal("err = nil, want the inner error")
	}
}

func TestRenderInSubqueryInnerError(t *testing.T) {
	t.Parallel()

	bad := nSub("widget_orders", []string{"widget_id"},
		Node{Kind: KindBinary, Column: "amount", Op: OpEqAny, Value: 1})

	_, _, err := renderExpr(sqlite.New(),
		Node{Kind: KindIn, Column: "id", Op: OpIn, Value: bad}, &argCounter{})
	if err == nil {
		t.Fatal("err = nil, want the inner error")
	}
}

func TestRenderEmptyInListIsAlwaysFalse(t *testing.T) {
	t.Parallel()

	clause, args, err := renderExpr(sqlite.New(),
		Node{Kind: KindIn, Column: "id", Op: OpIn, Value: []any{}}, &argCounter{})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	if clause != "1 = 0" {
		t.Fatalf("clause = %q, want 1 = 0", clause)
	}

	if args != nil {
		t.Fatalf("args = %#v, want nil", args)
	}
}

func TestRenderTupleInnerError(t *testing.T) {
	t.Parallel()

	bad := nSub("widget_orders", []string{"widget_id", "amount"},
		Node{Kind: KindBinary, Column: "amount", Op: OpEqAny, Value: 1})

	_, _, err := renderExpr(sqlite.New(), nTuple([]string{"id", "quantity"}, OpIn, bad), &argCounter{})
	if err == nil {
		t.Fatal("err = nil, want the inner error")
	}
}

func TestRenderCompoundErrorPaths(t *testing.T) {
	t.Parallel()

	bad := Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: 1}

	t.Run("not child error propagates", func(t *testing.T) {
		t.Parallel()

		_, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindCompound, Compound: CompoundNot, Children: []Node{bad}}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want the child error")
		}
	})

	t.Run("and child error propagates", func(t *testing.T) {
		t.Parallel()

		_, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindCompound, Compound: CompoundAnd, Children: []Node{
				{Kind: KindBinary, Column: "id", Op: OpEq, Value: 1},
				bad,
			}}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want the child error")
		}
	})

	t.Run("empty children are skipped", func(t *testing.T) {
		t.Parallel()

		clause, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindCompound, Compound: CompoundAnd, Children: []Node{
				{},
				{Kind: KindBinary, Column: "id", Op: OpEq, Value: 1},
			}}, &argCounter{})
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		if clause != `("id" = ?)` {
			t.Fatalf("clause = %q, want the surviving child only", clause)
		}
	})
}

func TestRenderFuncPredicateShapes(t *testing.T) {
	t.Parallel()

	coalesce := func() *FuncExpr { return fnNode("COALESCE", colLeaf("bio"), litLeaf("n/a")) }

	t.Run("is not null", func(t *testing.T) {
		t.Parallel()

		q, _, err := Select(sqlite.New(), "widgets", []string{"id"},
			Node{Kind: KindFunc, Func: coalesce(), Op: OpIsNotNull}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id" FROM "widgets" WHERE COALESCE("bio", ?) IS NOT NULL`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}
	})

	t.Run("empty in is always false", func(t *testing.T) {
		t.Parallel()

		q, args, err := Select(sqlite.New(), "widgets", []string{"id"},
			Node{Kind: KindFunc, Func: coalesce(), Op: OpIn, Value: []any{}}, nil, 0, 0)
		if err != nil {
			t.Fatalf("err = %v, want nil", err)
		}

		want := `SELECT "id" FROM "widgets" WHERE 1 = 0`
		if q != want {
			t.Fatalf("query = %q, want %q", q, want)
		}

		if !reflect.DeepEqual(args, []any{"n/a"}) {
			t.Fatalf("args = %#v, want the expression args", args)
		}
	})

	t.Run("invalid operator", func(t *testing.T) {
		t.Parallel()

		_, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindFunc, Func: coalesce(), Op: OpLike, Value: "x"}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want an operator error")
		}
	})

	t.Run("empty function name", func(t *testing.T) {
		t.Parallel()

		_, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindFunc, Func: &FuncExpr{Args: []Node{colLeaf("bio")}}, Op: OpEq, Value: "x"},
			&argCounter{})
		if err == nil {
			t.Fatal("err = nil, want an empty-name error")
		}
	})

	t.Run("argument scalar error propagates", func(t *testing.T) {
		t.Parallel()

		badArg := Node{Kind: KindBinary, Column: "bio", Op: OpEq, Value: "x"}
		fn := fnNode("LOWER", badArg)

		_, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindFunc, Func: fn, Op: OpEq, Value: "x"}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want the argument error")
		}
	})
}

func TestRenderScalarSubqueryShapes(t *testing.T) {
	t.Parallel()

	t.Run("non-subquery value rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := renderScalar(sqlite.New(),
			Node{Kind: KindSubquery, Value: "x"}, &argCounter{}, renderScope{})
		if err == nil {
			t.Fatal("err = nil, want a payload error")
		}
	})

	t.Run("multi-column projection rejected", func(t *testing.T) {
		t.Parallel()

		sq := nSub("orders", []string{"a", "b"}, Node{})

		_, _, err := renderScalar(sqlite.New(),
			Node{Kind: KindSubquery, Value: sq}, &argCounter{}, renderScope{})
		if err == nil {
			t.Fatal("err = nil, want an arity error")
		}
	})

	t.Run("inner error propagates", func(t *testing.T) {
		t.Parallel()

		sq := nSub("orders", []string{"a"},
			Node{Kind: KindBinary, Column: "a", Op: OpEqAny, Value: 1})

		_, _, err := renderScalar(sqlite.New(),
			Node{Kind: KindSubquery, Value: sq}, &argCounter{}, renderScope{})
		if err == nil {
			t.Fatal("err = nil, want the inner error")
		}
	})

	t.Run("unsupported argument kind rejected", func(t *testing.T) {
		t.Parallel()

		_, _, err := renderScalar(sqlite.New(),
			Node{Kind: KindBinary, Column: "bio", Op: OpEq, Value: "x"}, &argCounter{}, renderScope{})
		if err == nil {
			t.Fatal("err = nil, want a kind error")
		}
	})
}

func TestRenderCaseErrorPaths(t *testing.T) {
	t.Parallel()

	goodCond := nBinary("widgets", "active", OpEq, true)
	goodThen := Node{Kind: KindLit, Value: 1}

	t.Run("condition error propagates", func(t *testing.T) {
		t.Parallel()

		badCond := Node{Kind: KindBinary, Column: "active", Op: OpEqAny, Value: true}
		fn := &FuncExpr{Case: true, Whens: []FuncWhen{{Cond: badCond, Then: goodThen}}}

		_, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindFunc, Func: fn, Op: OpEq, Value: 1}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want the condition error")
		}
	})

	t.Run("empty condition rejected", func(t *testing.T) {
		t.Parallel()

		fn := &FuncExpr{Case: true, Whens: []FuncWhen{{Then: goodThen}}}

		_, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindFunc, Func: fn, Op: OpEq, Value: 1}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want an empty-condition error")
		}
	})

	t.Run("then error propagates", func(t *testing.T) {
		t.Parallel()

		badThen := Node{Kind: KindBinary, Column: "active", Op: OpEq, Value: true}
		fn := &FuncExpr{Case: true, Whens: []FuncWhen{{Cond: goodCond, Then: badThen}}}

		_, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindFunc, Func: fn, Op: OpEq, Value: 1}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want the THEN error")
		}
	})

	t.Run("else error propagates", func(t *testing.T) {
		t.Parallel()

		badElse := Node{Kind: KindBinary, Column: "active", Op: OpEq, Value: true}
		fn := &FuncExpr{Case: true, Whens: []FuncWhen{{Cond: goodCond, Then: goodThen}}, Else: &badElse}

		_, _, err := renderExpr(sqlite.New(),
			Node{Kind: KindFunc, Func: fn, Op: OpEq, Value: 1}, &argCounter{})
		if err == nil {
			t.Fatal("err = nil, want the ELSE error")
		}
	})
}

func TestSelectOrderFuncError(t *testing.T) {
	t.Parallel()

	_, _, err := Select(sqlite.New(), "widgets", []string{"id"}, Node{},
		[]OrderTerm{{Func: &FuncExpr{}}}, 0, 0)
	if err == nil {
		t.Fatal("err = nil, want the ORDER BY expression error")
	}
}

func TestSelectUnknownNullsOrder(t *testing.T) {
	t.Parallel()

	_, _, err := Select(postgres.New(), "widgets", []string{"id"}, Node{},
		[]OrderTerm{{Column: "bio", Nulls: NullsOrder(99)}}, 0, 0)
	if err == nil {
		t.Fatal("err = nil, want an unknown-nulls-order error")
	}
}

func TestCountPropagatesWhereError(t *testing.T) {
	t.Parallel()

	_, _, err := Count(sqlite.New(), "widgets",
		Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: 1})
	if err == nil {
		t.Fatal("err = nil, want the WHERE error")
	}
}
