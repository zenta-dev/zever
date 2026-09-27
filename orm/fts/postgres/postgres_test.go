package postgres

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	orm "github.com/zenta-dev/zever/orm"
)

// doc is a fixture entity with a searchable text column, mirroring the
// shape schema codegen generates for a `body: string` field.
type doc struct {
	ID   string
	Body string
}

func (d *doc) Scan(row orm.Row) error {
	return row.Scan(&d.ID, &d.Body)
}

var (
	docs    = orm.NewTable[doc]("docs", []string{"id", "body"})
	docBody = orm.NewColumn[doc, string]("docs", "body")
)

// captureExec is a db.DB that records the last query/args it was asked to
// run and returns an empty result set -- enough to assert the exact
// Postgres SQL the FTS package renders, without needing a live Postgres.
type captureExec struct {
	lastQuery string
	lastArgs  []any
}

func (c *captureExec) Query(_ context.Context, query string, args ...any) (db.Rows, error) {
	c.lastQuery = query
	c.lastArgs = args

	return emptyRows{}, nil
}

func (*captureExec) Exec(context.Context, string, ...any) (int64, error) { return 0, nil }
func (*captureExec) Ping(context.Context) error                          { return nil }
func (*captureExec) Close(context.Context) error                         { return nil }
func (*captureExec) Dialect() string                                     { return "postgres" }

type emptyRows struct{}

func (emptyRows) Next() bool                 { return false }
func (emptyRows) Scan(...any) error          { return nil }
func (emptyRows) Close() error               { return nil }
func (emptyRows) Err() error                 { return nil }
func (emptyRows) Columns() ([]string, error) { return nil, nil }

func TestPostgresFTSMatchRendering(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name     string
		pred     func() orm.Predicate[doc]
		wantSQL  string
		wantArgs []any
	}{
		{
			"match plain",
			func() orm.Predicate[doc] { return Match(docBody, "distributed systems") },
			`SELECT "id", "body" FROM "docs" WHERE to_tsvector('english', "body") @@ plainto_tsquery('english', $1)`,
			[]any{"distributed systems"},
		},
		{
			"match tsquery",
			func() orm.Predicate[doc] { return MatchTSQuery(docBody, "sql & \"acid\"") },
			`SELECT "id", "body" FROM "docs" WHERE to_tsvector('english', "body") @@ to_tsquery('english', $1)`,
			[]any{`sql & "acid"`},
		},
		{
			"match composed with and",
			func() orm.Predicate[doc] {
				return orm.And(Match(docBody, "go"), orm.Contains[doc](docBody, "iterator"))
			},
			`SELECT "id", "body" FROM "docs" WHERE (to_tsvector('english', "body") @@ plainto_tsquery('english', $1) AND "body" LIKE $2 ESCAPE '\')`,
			[]any{"go", "%iterator%"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			capture := &captureExec{}

			if _, err := orm.From(docs).Where(tc.pred()).All(ctx, capture); err != nil {
				t.Fatalf("All: %v", err)
			}

			if capture.lastQuery != tc.wantSQL {
				t.Fatalf("SQL = %q, want %q", capture.lastQuery, tc.wantSQL)
			}

			if !equalArgs(capture.lastArgs, tc.wantArgs) {
				t.Fatalf("args = %#v, want %#v", capture.lastArgs, tc.wantArgs)
			}
		})
	}
}

func TestPostgresFTSRankOrdering(t *testing.T) {
	ctx := t.Context()

	capture := &captureExec{}

	_, err := orm.From(docs).
		Where(Match(docBody, "go")).
		OrderBy(Rank(docBody, "go")).
		All(ctx, capture)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	want := `SELECT "id", "body" FROM "docs" WHERE to_tsvector('english', "body") @@ plainto_tsquery('english', $1) ORDER BY ts_rank(to_tsvector('english', "body"), plainto_tsquery('english', $2)) DESC`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !equalArgs(capture.lastArgs, []any{"go", "go"}) {
		t.Fatalf("args = %#v, want [go go]", capture.lastArgs)
	}
}

func TestPostgresFTSRankOrderingWithLimit(t *testing.T) {
	ctx := t.Context()

	capture := &captureExec{}

	_, err := orm.From(docs).
		Where(Match(docBody, "sql")).
		OrderBy(Rank(docBody, "sql")).
		Limit(5).
		All(ctx, capture)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// Placeholders stay sequential across WHERE, ORDER BY and LIMIT: the
	// WHERE query text is $1, the ORDER BY rank text $2, the LIMIT $3.
	want := `SELECT "id", "body" FROM "docs" WHERE to_tsvector('english', "body") @@ plainto_tsquery('english', $1) ORDER BY ts_rank(to_tsvector('english', "body"), plainto_tsquery('english', $2)) DESC LIMIT $3`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}

	if !equalArgs(capture.lastArgs, []any{"sql", "sql", 5}) {
		t.Fatalf("args = %#v, want [sql sql 5]", capture.lastArgs)
	}
}

func TestPostgresFTSMatchQualifiedInJoin(t *testing.T) {
	ctx := t.Context()

	capture := &captureExec{}

	rel := orm.NewRelation[doc, doc]("id", "id", docs)
	_, err := orm.JoinOn(orm.From(docs), rel, orm.InnerJoin).
		Where(Match(docBody, "go")).
		OrderBy(Rank(docBody, "go")).
		All(ctx, capture)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	// Join WHERE clauses and order terms are qualified to their own table,
	// and the FTS expressions carry the qualification into tsvector/ts_rank.
	want := `SELECT "docs"."id", "docs"."body", "docs"."id", "docs"."body" FROM "docs" INNER JOIN "docs" ON "docs"."id" = "docs"."id" WHERE to_tsvector('english', "docs"."body") @@ plainto_tsquery('english', $1) ORDER BY ts_rank(to_tsvector('english', "docs"."body"), plainto_tsquery('english', $2)) DESC`
	if capture.lastQuery != want {
		t.Fatalf("SQL = %q, want %q", capture.lastQuery, want)
	}
}

func equalArgs(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
