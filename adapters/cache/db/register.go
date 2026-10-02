package db

import (
	"github.com/zenta-dev/zever/core/cache"
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// Adapter is the DB-backed cache adapter name.
const Adapter cache.Adapter = cache.DB

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = cache.Register(Adapter, func(o cache.Options) (cache.Cache, error) {
		poolOpts := dbconn.SplitDSN(o.DSN)
		poolOpts.DedicatedPool = o.DedicatedPool
		return New(Options{Options: poolOpts})
	})
	_ = cache.RegisterShared(Adapter, func(conn coredb.DB, o cache.Options) (cache.Cache, error) {
		poolOpts := dbconn.SplitDSN(o.DSN)
		poolOpts.DedicatedPool = o.DedicatedPool
		return OpenFromDB(conn, Options{Options: poolOpts})
	})
}
