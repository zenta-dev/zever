package render

import (
	"reflect"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
)

// ftsNode builds an FTS predicate node with the same shape
// orm.NewFTSPredicate produces.
func ftsNode(op FTSOp, column string, query string) Node {
	return Node{Kind: KindFTS, Table: "docs", Column: column, FTS: &FTSExpr{Op: op, Query: query}}
}

func TestRenderFTSMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		node Node
		dial string
		want string
		args []any
	}{
		{
			"match plain / postgres",
			ftsNode(FTSMatch, "body", "distributed systems"),
			"postgres",
			`SELECT "id" FROM "docs" WHERE to_tsvector('english', "body") @@ plainto_tsquery('english', $1)`,
			[]any{"distributed systems"},
		},
		{
			"match plain / sqlite",
			ftsNode(FTSMatch, "body", "distributed systems"),
			"sqlite",
			`SELECT "id" FROM "docs" WHERE "body" MATCH ?`,
			[]any{"distributed systems"},
		},
		{
			"match tsquery / postgres",
			ftsNode(FTSMatchTSQuery, "body", `sql & "acid"`),
			"postgres",
			`SELECT "id" FROM "docs" WHERE to_tsvector('english', "body") @@ to_tsquery('english', $1)`,
			[]any{`sql & "acid"`},
		},
		{
			"match tsquery / sqlite renders plainto form",
			ftsNode(FTSMatchTSQuery, "body", "sql & acid"),
			"sqlite",
			// FTS5 has no to_tsquery; an FTSMatchTSQuery node never comes
			// from the sqlite package, and if one somehow reaches a sqlite
			// render the MATCH form is still correct SQL (the FTS5 MATCH
			// grammar accepts boolean queries natively).
			`SELECT "id" FROM "docs" WHERE "body" MATCH ?`,
			[]any{"sql & acid"},
		},
		{
			"match combined with another predicate",
			Node{
				Kind:     KindCompound,
				Compound: CompoundAnd,
				Children: []Node{
					ftsNode(FTSMatch, "title", "go"),
					{Kind: KindBinary, Table: "docs", Column: "draft", Op: OpEq, Value: true},
				},
			},
			"postgres",
			`SELECT "id" FROM "docs" WHERE (to_tsvector('english', "title") @@ plainto_tsquery('english', $1) AND "draft" = $2)`,
			[]any{"go", true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q, args, err := Select(dialectFor(tc.dial), "docs", []string{"id"}, tc.node, nil, 0, 0)
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

func TestRenderFTSRankOrderBy(t *testing.T) {
	t.Parallel()
	order := []OrderTerm{
		{Column: "body", Desc: true, FTS: &FTSExpr{Op: FTSRank, Query: "go"}},
	}

	q, args, err := Select(postgres.New(), "docs", []string{"id"}, ftsNode(FTSMatch, "body", "go"), order, 10, 5)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "docs" WHERE to_tsvector('english', "body") @@ plainto_tsquery('english', $1) ORDER BY ts_rank(to_tsvector('english', "body"), plainto_tsquery('english', $2)) DESC LIMIT $3 OFFSET $4`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"go", "go", 10, 5}) {
		t.Fatalf("args = %#v, want [go go 10 5]", args)
	}
}

func TestRenderFTSRankOrderByAsc(t *testing.T) {
	t.Parallel()
	order := []OrderTerm{
		{Column: "body", FTS: &FTSExpr{Op: FTSRank, Query: "sql"}},
	}

	q, _, err := Select(postgres.New(), "docs", []string{"id"}, Node{}, order, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "docs" ORDER BY ts_rank(to_tsvector('english', "body"), plainto_tsquery('english', $1)) ASC`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

func TestRenderFTSQualifiedInJoin(t *testing.T) {
	t.Parallel()
	// An FTS predicate used as a join's WHERE must be qualified to its own
	// table, exactly like a plain column predicate.
	where := qualifyNode(ftsNode(FTSMatch, "body", "go"), "")

	q, args, err := Select(postgres.New(), "docs", []string{"id"}, where, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	want := `SELECT "id" FROM "docs" WHERE to_tsvector('english', "docs"."body") @@ plainto_tsquery('english', $1)`
	if q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if !reflect.DeepEqual(args, []any{"go"}) {
		t.Fatalf("args = %#v, want [go]", args)
	}
}

// TestRenderFTSNilPayload proves a KindFTS node with no FTS expression
// renders no clause rather than failing or emitting broken SQL.
func TestRenderFTSNilPayload(t *testing.T) {
	t.Parallel()

	q, _, err := Select(postgres.New(), "docs", []string{"id"}, Node{Kind: KindFTS}, nil, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	if want := `SELECT "id" FROM "docs"`; q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}
}

// TestRenderFTSNonRankOrderTerm proves an order term carrying a non-rank FTS
// expression renders the plain quoted column, binding nothing.
func TestRenderFTSNonRankOrderTerm(t *testing.T) {
	t.Parallel()

	order := []OrderTerm{{Column: "body", FTS: &FTSExpr{Op: FTSMatch, Query: "go"}}}

	q, args, err := Select(postgres.New(), "docs", []string{"id"}, Node{}, order, 0, 0)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}

	if want := `SELECT "id" FROM "docs" ORDER BY "body" ASC`; q != want {
		t.Fatalf("query = %q, want %q", q, want)
	}

	if len(args) != 0 {
		t.Fatalf("args = %#v, want none", args)
	}
}
