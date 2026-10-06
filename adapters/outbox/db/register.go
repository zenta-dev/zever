package db

import (
	coredb "github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// Adapter is the DB-backed outbox adapter name.
const Adapter outbox.Adapter = outbox.DB

// Register wires this adapter into its battery registry. Call from your app's
// main or generated app.go; no init magic. Core outbox.Options carry no
// Publisher, so a store opened this way records messages but cannot Start its
// relay until application wiring supplies one through New/OpenFromDB.
func Register() {
	_ = outbox.Register(Adapter, func(o outbox.Options) (outbox.Store, error) {
		return New(optionsFrom(o))
	})
	_ = outbox.RegisterShared(Adapter, func(conn coredb.DB, o outbox.Options) (outbox.Store, error) {
		return OpenFromDB(conn, optionsFrom(o))
	})
}

// optionsFrom maps core outbox.Options onto the adapter Options. The
// Publisher interface is intentionally unset: core carries only the
// informational string selector, which is carried over as Transport for the
// messaging.system span attribute.
func optionsFrom(o outbox.Options) Options {
	poolOpts := dbconn.SplitDSN(o.DSN)
	poolOpts.DedicatedPool = o.DedicatedPool

	return Options{
		Options:      poolOpts,
		Table:        o.Table,
		InboxTable:   o.InboxTable,
		Transport:    o.Publisher,
		PollInterval: o.PollInterval,
		BatchSize:    o.BatchSize,
		MaxAttempts:  o.MaxAttempts,
		Retry:        o.Retry,
		Retention:    o.Retention,
		LockSeconds:  o.LockSeconds,
		Provider:     o.Provider,
	}
}
