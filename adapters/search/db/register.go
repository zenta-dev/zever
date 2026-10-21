package db

import (
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// Adapter is the canonical DB search adapter name.
const Adapter search.Adapter = search.DB

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// The driver picks its backend from the DSN: an empty DSN selects the
// embedded sqlite backend, a postgres URL selects PostgreSQL. The adapter
// is registered under one name, search.DB ("db").
func Register() {
	_ = search.Register(search.DB, func(o search.Options) (search.Search, error) {
		poolOpts := dbconn.SplitDSN(o.DSN)
		poolOpts.DedicatedPool = o.DedicatedPool
		return New(Options{Options: poolOpts})
	})
	_ = search.RegisterShared(search.DB, func(conn coredb.DB, o search.Options) (search.Search, error) {
		poolOpts := dbconn.SplitDSN(o.DSN)
		poolOpts.DedicatedPool = o.DedicatedPool
		return OpenFromDB(conn, Options{Options: poolOpts})
	})
}
