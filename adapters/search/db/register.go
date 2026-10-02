package db

import (
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// Adapter is the canonical DB search adapter name. The legacy "postgres"
// and "sqlite" names stay registered as aliases so existing zever.yaml
// files keep resolving.
const Adapter search.Adapter = search.DB

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// The driver serves every registered name from one build: an empty DSN
// selects the embedded sqlite backend. New projects should select
// search.DB ("db").
func Register() {
	for _, name := range []search.Adapter{search.DB, search.Postgres, search.SQLite} {
		name := name
		_ = search.Register(name, func(o search.Options) (search.Search, error) {
			poolOpts := dbconn.SplitDSN(o.DSN)
			poolOpts.DedicatedPool = o.DedicatedPool
			return New(Options{Options: poolOpts})
		})
	}
	shared := func(conn coredb.DB, o search.Options) (search.Search, error) {
		poolOpts := dbconn.SplitDSN(o.DSN)
		poolOpts.DedicatedPool = o.DedicatedPool
		return OpenFromDB(conn, Options{Options: poolOpts})
	}
	_ = search.RegisterShared(search.DB, shared)
	_ = search.RegisterShared(search.Postgres, shared)
	_ = search.RegisterShared(search.SQLite, shared)
}
