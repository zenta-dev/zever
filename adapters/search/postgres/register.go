package postgres

import (
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// Adapter is the postgres search adapter name.
const Adapter search.Adapter = search.Postgres

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// The driver serves both search.Postgres and search.SQLite: an empty DSN
// selects the embedded sqlite backend, so the legacy sqlite adapter name
// keeps resolving after adapters/search/sqlite was folded into this
// package. New projects should select search.Postgres.
func Register() {
	_ = search.Register(search.Postgres, func(o search.Options) (search.Search, error) {
		poolOpts := dbconn.SplitDSN(o.DSN)
		poolOpts.DedicatedPool = o.DedicatedPool
		return New(Options{Options: poolOpts})
	})
	_ = search.Register(search.SQLite, func(o search.Options) (search.Search, error) {
		poolOpts := dbconn.SplitDSN(o.DSN)
		poolOpts.DedicatedPool = o.DedicatedPool
		return New(Options{Options: poolOpts})
	})
}
