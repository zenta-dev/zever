package postgres

import (
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// Adapter is the postgres scheduler adapter name.
const Adapter scheduler.Adapter = "postgres"

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// The registered factory builds a sqlite-backed driver by default (an
// empty DSN selects ":memory:"); durable multi-instance deploys set DSN to
// a postgres URL (or a sqlite file path), distinct Owners per replica, and
// the shared dispatcher.
func Register() {
	_ = scheduler.Register(Adapter, func(o scheduler.Options) (scheduler.Scheduler, error) {
		poolOpts := dbconn.SplitDSN(o.DSN)
		poolOpts.DedicatedPool = o.DedicatedPool
		return New(Options{Options: o, PoolOptions: poolOpts})
	})
}
