package db

import (
	"errors"
	"slices"
	"testing"

	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
)

// removedWorkflowAliases are the adapter names this package used to register
// next to the canonical "db" name. They only ever pointed at the same
// driver, so they are gone: a set DSN selects PostgreSQL, an empty one
// selects embedded SQLite.
var removedWorkflowAliases = []string{"postgres"}

// TestRegister_canonicalNameOnly pins the break: the DB workflow adapter is
// registered under "db" only, and the removed alias name no longer resolves
// through the registry.
func TestRegister_canonicalNameOnly(t *testing.T) {
	Register()

	names := workflow.RegisteredAdapters()
	if !slices.Contains(names, string(workflow.DB)) {
		t.Errorf("RegisteredAdapters() = %v, want it to contain %q", names, workflow.DB)
	}

	for _, alias := range removedWorkflowAliases {
		if slices.Contains(names, alias) {
			t.Errorf("RegisteredAdapters() = %v, want no %q entry", names, alias)
		}
	}

	conn, err := dbsqlite.New(coredb.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("dbsqlite.New() err = %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(t.Context()) })

	for _, alias := range removedWorkflowAliases {
		if _, err := workflow.Open(workflow.Adapter(alias), workflow.Options{}); !errors.Is(err, workflow.ErrUnknownAdapter) {
			t.Errorf("Open(%q) err = %v, want ErrUnknownAdapter", alias, err)
		}

		if _, err := workflow.OpenShared(workflow.Adapter(alias), conn, workflow.Options{}); !errors.Is(err, workflow.ErrUnknownAdapter) {
			t.Errorf("OpenShared(%q) err = %v, want ErrUnknownAdapter", alias, err)
		}
	}
}
