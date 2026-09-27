// Command app is the full-text-search example: a searchable document
// entity (schema/fts.zen, codegen'd by the zenorm backend into
// generated/zenorm/orm/gen/app), an FTS5 virtual table over the same
// columns (created via raw DDL — the FTS5 table shape is not expressible in
// the .zen DSL), and queries built through orm/fts/sqlite's typed helpers.
//
// It seeds four documents, searches them with an FTS5 MATCH predicate,
// re-runs the search ordered by FTS5 relevance (the hidden rank column),
// and counts matches — every query a normal orm.From(...).Where(...) built
// with the fts package's Match/Rank helpers, no raw SQL at the call site.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/adapters/db/sqlite"
	"github.com/zenta-dev/zever/core/db"
	gen "github.com/zenta-dev/zever/examples/fts/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
	ftssqlite "github.com/zenta-dev/zever/orm/fts/sqlite"
)

func main() {
	ctx := context.Background()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		die(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	if err := seed(ctx, conn); err != nil {
		die(err)
	}

	if err := search(ctx, conn); err != nil {
		die(err)
	}

	if err := rankedSearch(ctx, conn); err != nil {
		die(err)
	}

	if err := countMatches(ctx, conn); err != nil {
		die(err)
	}
}

// seed creates the FTS5 virtual table and inserts four documents. The
// virtual table is created via raw DDL (the FTS5 shape is a migration
// concern, not expressible in the .zen DSL); the inserts use
// orm.Insert/orm.Set so the seed data flows through the same typed surface
// as everything else.
func seed(ctx context.Context, conn db.DB) error {
	// fts5(id, title, body) indexes every column; column-scoped MATCH (e.g.
	// the body search below) works against any of them.
	if _, err := conn.Exec(ctx, `CREATE VIRTUAL TABLE docs USING fts5(id, title, body)`); err != nil {
		return fmt.Errorf("create fts5 table: %w", err)
	}

	// (id, title, body)
	docs := []struct {
		id, title, body string
	}{
		{"d1", "Go release", "Go 1.27 ships generators and iterators"},
		{"d2", "Rust borrow checker", "memory safety without a garbage collector"},
		{"d3", "Distributed systems", "consensus, replication and the cap theorem"},
		{"d4", "Go on the web", "writing go http services with the go standard library"},
	}

	for _, d := range docs {
		insert := orm.InsertInto(gen.Docs).Values(
			orm.Set(gen.DocCols.ID, d.id),
			orm.Set(gen.DocCols.Title, d.title),
			orm.Set(gen.DocCols.Body, d.body),
		)

		if err := insert.Exec(ctx, conn); err != nil {
			return fmt.Errorf("insert %s: %w", d.id, err)
		}
	}

	return nil
}

// search runs a full-text MATCH against the body column and prints the
// matching titles.
func search(ctx context.Context, conn db.DB) error {
	hits, err := orm.From(gen.Docs).
		Where(ftssqlite.Match(gen.DocCols.Body, "go")).
		OrderBy(gen.DocCols.ID.Asc()).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("fts5 match: %w", err)
	}

	fmt.Printf("body matches \"go\": %s\n", titles(hits))

	return nil
}

// rankedSearch re-runs the same MATCH ordered by FTS5 relevance — the
// fts5 hidden rank column — proving Rank composes into OrderBy.
func rankedSearch(ctx context.Context, conn db.DB) error {
	hits, err := orm.From(gen.Docs).
		Where(ftssqlite.Match(gen.DocCols.Body, "go")).
		OrderBy(ftssqlite.Rank(gen.DocCols.Body)).
		All(ctx, conn)
	if err != nil {
		return fmt.Errorf("fts5 ranked match: %w", err)
	}

	fmt.Println("body matches \"go\", most relevant first:")

	for _, d := range hits {
		fmt.Printf("  %s %q\n", d.ID, d.Title)
	}

	return nil
}

// countMatches proves Count works over the same FTS5 MATCH predicate.
func countMatches(ctx context.Context, conn db.DB) error {
	n, err := orm.From(gen.Docs).Where(ftssqlite.Match(gen.DocCols.Body, "go")).Count(ctx, conn)
	if err != nil {
		return fmt.Errorf("fts5 count: %w", err)
	}

	fmt.Printf("body matches \"go\": count = %d\n", n)

	return nil
}

func titles(docs []*gen.Doc) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = fmt.Sprintf("%s %q", d.ID, d.Title)
	}

	return out
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
