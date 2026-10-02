package db

import (
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// Adapter is the DB-backed idempotency adapter name.
const Adapter idempotency.Adapter = idempotency.Adapter("db")

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = idempotency.Register(Adapter, func(o idempotency.Options) (idempotency.Store, error) {
		poolOpts := dbconn.SplitDSN(o.DSN)
		poolOpts.DedicatedPool = o.DedicatedPool
		return New(Options{Options: poolOpts, TTL: o.TTL})
	})
}
