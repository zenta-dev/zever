package cdc

import (
	"github.com/zenta-dev/zever/core/outbox"
)

// Adapter is the CDC outbox adapter name.
const Adapter outbox.Adapter = outbox.CDC

// Register wires this adapter into its battery registry. Call from your app's
// main or generated app.go; no init magic. Core outbox.Options carry no
// Publisher, so a store opened this way records messages but cannot Start its
// consumer until application wiring supplies one through New.
func Register() {
	_ = outbox.Register(Adapter, func(o outbox.Options) (outbox.Store, error) {
		return New(Options{Options: o})
	})
}
