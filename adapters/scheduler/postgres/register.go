package postgres

import (
	"github.com/zenta-dev/zever/core/scheduler"
)

// Adapter is the postgres scheduler adapter name.
const Adapter scheduler.Adapter = "postgres"

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// The registered factory builds a sqlite-backed driver by default (the
// zero scheduler.Options plus an empty DSN selects ":memory:"); durable
// multi-instance deploys construct the driver directly with New and a
// postgres DSN, distinct Owners per replica, and the shared dispatcher.
func Register() {
	_ = scheduler.Register(Adapter, func(o scheduler.Options) (scheduler.Scheduler, error) {
		return New(Options{Options: o})
	})
}
