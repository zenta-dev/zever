package db

import (
	"github.com/zenta-dev/zever/core/session"
)

// Adapter is the DB-backed session adapter name.
const Adapter session.Adapter = session.Adapter("db")

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = session.Register(Adapter, func(o session.Options) (session.Store, error) {
		return New(Options{TTL: o.TTL})
	})
}
