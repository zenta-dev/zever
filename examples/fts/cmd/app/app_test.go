package main

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
	gen "github.com/zenta-dev/zever/examples/fts/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
	ftssqlite "github.com/zenta-dev/zever/orm/fts/sqlite"
)

func newTestDB(ctx context.Context, t *testing.T) db.DB {
	t.Helper()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })

	if err := seed(ctx, conn); err != nil {
		t.Fatalf("seed: %v", err)
	}

	return conn
}

func TestMatchSet(t *testing.T) {
	ctx := t.Context()
	conn := newTestDB(ctx, t)

	got, err := orm.From(gen.Docs).
		Where(ftssqlite.Match(gen.DocCols.Body, "go")).
		OrderBy(gen.DocCols.ID.Asc()).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != 2 || got[0].ID != "d1" || got[1].ID != "d4" {
		t.Fatalf("match set = %v, want [d1 d4]", ids(got))
	}
}

func TestRankOrder(t *testing.T) {
	ctx := t.Context()
	conn := newTestDB(ctx, t)

	got, err := orm.From(gen.Docs).
		Where(ftssqlite.Match(gen.DocCols.Body, "go")).
		OrderBy(ftssqlite.Rank(gen.DocCols.Body)).
		All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(got) != 2 || got[0].ID != "d1" || got[1].ID != "d4" {
		t.Fatalf("rank order = %v, want [d1 d4]", ids(got))
	}
}

func TestMatchCount(t *testing.T) {
	ctx := t.Context()
	conn := newTestDB(ctx, t)

	n, err := orm.From(gen.Docs).Where(ftssqlite.Match(gen.DocCols.Body, "go")).Count(ctx, conn)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
}

func ids(docs []*gen.Doc) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.ID
	}
	return out
}
