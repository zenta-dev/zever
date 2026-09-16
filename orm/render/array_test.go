package render

import (
	"errors"
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect"
	"github.com/zenta-dev/zever/orm/dialect/postgres"
)

// arrayNode builds a KindArray predicate node with the same shape
// Column.EqAny/NeqAny/EqAll/NeqAll produce.
func arrayNode(op Op, column string, values ...any) Node {
	return Node{Kind: KindArray, Table: "users", Column: column, Op: op, Value: values}
}

func TestRenderArrayPredicate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		node Node
		want string
		args []any
	}{
		{
			"=",
			arrayNode(OpEqAny, "age", 18, 21),
			`SELECT "id" FROM "users" WHERE "age" = ANY(ARRAY[$1, $2])`,
			[]any{18, 21},
		},
		{
			"<> ANY",
			arrayNode(OpNeqAny, "age", 18, 21),
			`SELECT "id" FROM "users" WHERE "age" <> ANY(ARRAY[$1, $2])`,
			[]any{18, 21},
		},
		{
			"= ALL",
			arrayNode(OpEqAll, "age", 18, 21),
			`SELECT "id" FROM "users" WHERE "age" = ALL(ARRAY[$1, $2])`,
			[]any{18, 21},
		},
		{
			"<> ALL",
			arrayNode(OpNeqAll, "age", 18, 21),
			`SELECT "id" FROM "users" WHERE "age" <> ALL(ARRAY[$1, $2])`,
			[]any{18, 21},
		},
		{
			"= ANY empty is always false",
			arrayNode(OpEqAny, "age"),
			`SELECT "id" FROM "users" WHERE 1 = 0`,
			nil,
		},
		{
			"<> ANY empty is always false",
			arrayNode(OpNeqAny, "age"),
			`SELECT "id" FROM "users" WHERE 1 = 0`,
			nil,
		},
		{
			"= ALL empty is always true",
			arrayNode(OpEqAll, "score"),
			`SELECT "id" FROM "users" WHERE 1 = 1`,
			nil,
		},
		{
			"<> ALL empty is always true",
			arrayNode(OpNeqAll, "age"),
			`SELECT "id" FROM "users" WHERE 1 = 1`,
			nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q, args, err := Select(postgres.New(), "users", []string{"id"}, tc.node, nil, 0, 0)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if q != tc.want {
				t.Fatalf("query = %q, want %q", q, tc.want)
			}

			if !reflect.DeepEqual(args, tc.args) {
				t.Fatalf("args = %#v, want %#v", args, tc.args)
			}
		})
	}
}

func TestRenderArrayPredicateCapabilityGate(t *testing.T) {
	t.Parallel()

	_, _, err := Select(dialectFor("sqlite"), "users", []string{"id"}, arrayNode(OpEqAny, "age", 1), nil, 0, 0)
	if !errors.Is(err, dialect.ErrUnsupportedByDialect) {
		t.Fatalf("err = %v, want errors.Is(err, dialect.ErrUnsupportedByDialect)", err)
	}
}

func TestRenderArrayPredicateUnknownOpFailsClosed(t *testing.T) {
	t.Parallel()
	_, _, err := Select(postgres.New(), "users", []string{"id"}, arrayNode(OpEq, "age", 1), nil, 0, 0)
	if err == nil {
		t.Fatal("err = nil, want a typed error for an operator that is not an array quantifier")
	}
}

func TestRenderArrayPlaceholderOrdering(t *testing.T) {
	t.Parallel()
	where := Node{
		Kind:     KindCompound,
		Compound: CompoundAnd,
		Children: []Node{
			arrayNode(OpEqAny, "age", 18, 21),
			{Kind: KindBinary, Table: "users", Column: "active", Op: OpEq, Value: true},
		},
	}

	q, args, err := Select(postgres.New(), "users", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "users" WHERE ("age" = ANY(ARRAY[$1, $2]) AND "active" = $3)`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{18, 21, true}) {
		t.Fatalf("args = %#v, want [18 21 true]", args)
	}
}

func TestRenderArrayPredicateNonSliceValueFailsClosed(t *testing.T) {
	t.Parallel()

	_, _, err := Select(postgres.New(), "users", []string{"id"}, arrayNode(OpEqAny, "age", 1), nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	bad := Node{Kind: KindArray, Table: "users", Column: "age", Op: OpEqAny, Value: "not-a-slice"}

	_, _, err = Select(postgres.New(), "users", []string{"id"}, bad, nil, 0, 0)
	if err == nil {
		t.Fatal("err = nil, want a non-[]any payload error")
	}
}
