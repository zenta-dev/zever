package search_test

import (
	"context"

	"github.com/zenta-dev/zever/search"
	searchsqlite "github.com/zenta-dev/zever/search/sqlite"
)

// ExampleOpen opens the sqlite search backend on a private in-memory database.
func ExampleOpen() {
	_ = search.Register(search.SQLite, searchsqlite.New)

	s, err := search.Open(search.SQLite, search.Options{DSN: ":memory:"})
	if err != nil {
		return
	}

	defer func() { _ = s.Close() }()
}

// ExampleSearch_IndexBatch indexes several documents in one call instead of
// one round trip per document.
func ExampleSearch_IndexBatch() {
	_ = search.Register(search.SQLite, searchsqlite.New)

	s, err := search.Open(search.SQLite, search.Options{DSN: ":memory:"})
	if err != nil {
		return
	}

	defer func() { _ = s.Close() }()

	err = s.IndexBatch(context.Background(), []search.Document{
		{ID: "doc-1", Index: "articles", Content: "first article"},
		{ID: "doc-2", Index: "articles", Content: "second article"},
	})
	if err != nil {
		return
	}
}
