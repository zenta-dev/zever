package db

import (
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
)

// Adapter is the canonical DB workflow adapter name. The legacy
// "postgres" name stays registered as an alias so existing zever.yaml
// files keep resolving.
const Adapter workflow.Adapter = workflow.DB

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// New projects should select workflow.DB ("db").
func Register() {
	for _, name := range []workflow.Adapter{workflow.DB, workflow.Postgres} {
		name := name
		_ = workflow.Register(name, func(o workflow.Options) (workflow.Workflow, error) {
			return New(Options{Options: coredb.Options{DSN: o.DSN, DedicatedPool: o.DedicatedPool}, Table: o.Table})
		})
		_ = workflow.RegisterShared(name, func(conn coredb.DB, o workflow.Options) (workflow.Workflow, error) {
			return OpenFromDB(conn, Options{Options: coredb.Options{DSN: o.DSN, DedicatedPool: o.DedicatedPool}, Table: o.Table})
		})
	}
}
