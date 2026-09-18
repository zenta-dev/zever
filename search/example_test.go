package search_test

import (
	"github.com/zenta-dev/zever/search"
	searchsqlite "github.com/zenta-dev/zever/search/sqlite"
)

// ExampleOpen opens the sqlite search backend on a private in-memory database.
func ExampleOpen() {
	_ = search.Register(search.SQLite, searchsqlite.Open)

	s, err := search.Open(search.SQLite, search.Options{DSN: ":memory:"})
	if err != nil {
		return
	}

	defer func() { _ = s.Close() }()
}
