package render

import (
	"reflect"
	"testing"
)

// TestRenderCrossTVF pins the DUAL-purpose TVF SELECT: an explicit qualified
// column list from BOTH the left table and the table-valued source, a
// `CROSS JOIN <fn>(...) AS <alias>` clause, a combined WHERE, per-side ORDER
// BY and LIMIT. render itself does not gate the source (the gate lives in the
// dialect package's RenderSource); these tests pin the emitted SQL and the
// placeholder/arg ordering.
func TestRenderCrossTVF(t *testing.T) {
	t.Parallel()
	q, args, err := SelectTVF(
		fakePostgres{},
		false,
		"widgets", []string{"id", "bio"},
		`json_each("widgets"."bio")`, "je", []string{"key", "value"},
		Node{},
		nBinary("je", "value", OpEq, "go"),
		[]OrderTerm{{Column: "id"}},
		[]OrderTerm{{Column: "key"}},
		5, 10,
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "widgets"."id", "widgets"."bio", "je"."key", "je"."value" ` +
		`FROM "widgets" CROSS JOIN json_each("widgets"."bio") AS "je" ` +
		`WHERE "je"."value" = $1 ORDER BY "widgets"."id" ASC, "je"."key" ASC LIMIT $2 OFFSET $3`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"go", 5, 10}) {
		t.Fatalf("args = %#v, want [go 5 10]", args)
	}
}

// TestRenderLeftTVF pins the null-safe LEFT JOIN shape: the mandatory ON
// clause is the constant TRUE, so an outer row with no source match is
// preserved with the source columns all NULL.
func TestRenderLeftTVF(t *testing.T) {
	t.Parallel()
	q, args, err := SelectTVF(
		fakePostgres{},
		true,
		"widgets", []string{"id"},
		`json_each("widgets"."bio")`, "je", []string{"value"},
		nBinary("widgets", "id", OpEq, "w1"),
		Node{},
		nil, nil,
		0, 0,
	)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "widgets"."id", "je"."value" ` +
		`FROM "widgets" LEFT JOIN json_each("widgets"."bio") AS "je" ON TRUE ` +
		`WHERE "widgets"."id" = $1`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"w1"}) {
		t.Fatalf("args = %#v, want [w1]", args)
	}
}

func TestRenderTVFErrorPaths(t *testing.T) {
	t.Parallel()

	t.Run("where error propagates", func(t *testing.T) {
		t.Parallel()

		bad := Node{Kind: KindBinary, Column: "id", Op: OpEqAny, Value: "w1"}

		_, _, err := SelectTVF(fakePostgres{}, false,
			"widgets", []string{"id"},
			`json_each("widgets"."bio")`, "je", []string{"value"},
			bad, Node{}, nil, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the WHERE error")
		}
	})

	t.Run("order error propagates", func(t *testing.T) {
		t.Parallel()

		badOrder := []OrderTerm{{Func: &FuncExpr{}}}

		_, _, err := SelectTVF(fakePostgres{}, false,
			"widgets", []string{"id"},
			`json_each("widgets"."bio")`, "je", []string{"value"},
			Node{}, Node{}, badOrder, nil, 0, 0)
		if err == nil {
			t.Fatal("err = nil, want the ORDER BY error")
		}
	})
}
