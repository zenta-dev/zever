package db

import (
	"errors"
	"slices"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/vectorstore"
)

// removedVectorStoreAliases are the adapter names this package used to
// register next to the canonical "db" name. They only ever pointed at the
// same driver, so they are gone: a postgres URL in the DSN selects
// PostgreSQL pgvector, anything else selects embedded SQLite.
var removedVectorStoreAliases = []string{"pgvector", "sqlite"}

// TestRegister_canonicalNameOnly pins the break: the DB vectorstore adapter
// is registered under "db" only, and the removed alias names no longer
// resolve through the registry.
func TestRegister_canonicalNameOnly(t *testing.T) {
	Register()

	names := vectorstore.RegisteredAdapters()
	if !slices.Contains(names, string(vectorstore.DB)) {
		t.Errorf("RegisteredAdapters() = %v, want it to contain %q", names, vectorstore.DB)
	}

	for _, alias := range removedVectorStoreAliases {
		if slices.Contains(names, alias) {
			t.Errorf("RegisteredAdapters() = %v, want no %q entry", names, alias)
		}
	}

	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	for _, alias := range removedVectorStoreAliases {
		if _, err := vectorstore.Open(vectorstore.Adapter(alias), vectorstore.Options{}); !errors.Is(err, vectorstore.ErrUnknownAdapter) {
			t.Errorf("Open(%q) err = %v, want ErrUnknownAdapter", alias, err)
		}

		if _, err := vectorstore.OpenShared(vectorstore.Adapter(alias), conn, vectorstore.Options{}); !errors.Is(err, vectorstore.ErrUnknownAdapter) {
			t.Errorf("OpenShared(%q) err = %v, want ErrUnknownAdapter", alias, err)
		}
	}
}
