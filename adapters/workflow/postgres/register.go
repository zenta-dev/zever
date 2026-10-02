package postgres

import (
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
)

// Adapter is the postgres workflow adapter name.
const Adapter workflow.Adapter = workflow.Postgres

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = workflow.Register(Adapter, func(o workflow.Options) (workflow.Workflow, error) {
		return New(Options{Options: coredb.Options{DSN: o.DSN, DedicatedPool: o.DedicatedPool}, Table: o.Table})
	})
}
