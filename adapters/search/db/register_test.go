package db

import (
	"errors"
	"slices"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/search"
)

// removedSearchAliases are the adapter names this package used to register
// next to the canonical "db" name. They only ever pointed at the same
// driver, so they are gone: a postgres URL in the DSN selects PostgreSQL,
// anything else selects embedded SQLite.
var removedSearchAliases = []string{"postgres", "sqlite"}

// TestRegister_canonicalNameOnly pins the break: the DB search adapter is
// registered under "db" only, and the removed alias names no longer resolve
// through the registry.
func TestRegister_canonicalNameOnly(t *testing.T) {
	Register()

	names := search.RegisteredAdapters()
	if !slices.Contains(names, string(search.DB)) {
		t.Errorf("RegisteredAdapters() = %v, want it to contain %q", names, search.DB)
	}

	for _, alias := range removedSearchAliases {
		if slices.Contains(names, alias) {
			t.Errorf("RegisteredAdapters() = %v, want no %q entry", names, alias)
		}
	}

	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	for _, alias := range removedSearchAliases {
		if _, err := search.Open(search.Adapter(alias), search.Options{}); !errors.Is(err, search.ErrUnknownAdapter) {
			t.Errorf("Open(%q) err = %v, want ErrUnknownAdapter", alias, err)
		}

		if _, err := search.OpenShared(search.Adapter(alias), conn, search.Options{}); !errors.Is(err, search.ErrUnknownAdapter) {
			t.Errorf("OpenShared(%q) err = %v, want ErrUnknownAdapter", alias, err)
		}
	}
}
