package render

import (
	"testing"

	sqlited "github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestSelectJoin3MixedRendersFlatTwoKinds pins the flat mixed-chain SQL: two
// JOIN clauses in order, each carrying its OWN keyword (INNER then LEFT),
// shared ON/WHERE/ORDER BY/LIMIT machinery identical to SelectJoin3.
func TestSelectJoin3MixedRendersFlatTwoKinds(t *testing.T) {
	t.Parallel()
	resetShapeCache()

	q, _, err := SelectJoin3Mixed(sqlited.New(), InnerJoin, LeftJoin,
		"a", []string{"id"}, "b", []string{"id"}, "c", []string{"id"},
		"id", "a_id", "id", "b_id",
		Node{}, Node{}, Node{}, nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("SelectJoin3Mixed: %v", err)
	}

	want := `SELECT "a"."id", "b"."id", "c"."id" FROM "a" INNER JOIN "b" ON "a"."id" = "b"."a_id" LEFT JOIN "c" ON "b"."id" = "c"."b_id"`
	if q != want {
		t.Fatalf("SQL:\n got %q\nwant %q", q, want)
	}
}

// TestSelectJoin3NestedRendersParenthesizedComposite pins the nested
// mixed-chain SQL that preserves the outer join's "keep every A row"
// semantics: the B/C pair is rendered as a parenthesized joined table and
// the outer ON clause joins A to it. This is deliberately NOT the flat
// left-associative chain, which SQL would evaluate as (A LEFT JOIN B) INNER
// JOIN C and thereby silently drop every A row with no B.
func TestSelectJoin3NestedRendersParenthesizedComposite(t *testing.T) {
	t.Parallel()
	resetShapeCache()

	q, _, err := SelectJoin3Nested(sqlited.New(), LeftJoin, InnerJoin,
		"a", []string{"id"}, "b", []string{"id"}, "c", []string{"id"},
		"id", "a_id", "id", "b_id",
		Node{}, Node{}, Node{}, nil, nil, nil, 0, 0)
	if err != nil {
		t.Fatalf("SelectJoin3Nested: %v", err)
	}

	want := `SELECT "a"."id", "b"."id", "c"."id" FROM "a" LEFT JOIN ("b" INNER JOIN "c" ON "b"."id" = "c"."b_id") ON "a"."id" = "b"."a_id"`
	if q != want {
		t.Fatalf("SQL:\n got %q\nwant %q", q, want)
	}
}
