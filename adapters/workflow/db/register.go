package db

import (
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/workflow"
)

// Adapter is the canonical DB workflow adapter name.
const Adapter workflow.Adapter = workflow.DB

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// The driver picks its backend from the DSN: an empty DSN selects the
// embedded sqlite backend, a set DSN selects PostgreSQL. The adapter is
// registered under one name, workflow.DB ("db").
func Register() {
	_ = workflow.Register(workflow.DB, func(o workflow.Options) (workflow.Workflow, error) {
		return New(Options{Options: coredb.Options{DSN: o.DSN, DedicatedPool: o.DedicatedPool}, Table: o.Table})
	})
	_ = workflow.RegisterShared(workflow.DB, func(conn coredb.DB, o workflow.Options) (workflow.Workflow, error) {
		return OpenFromDB(conn, Options{Options: coredb.Options{DSN: o.DSN, DedicatedPool: o.DedicatedPool}, Table: o.Table})
	})
}
