package db

import (
	"github.com/zenta-dev/zever/core/queue"
)

// Adapter is the DB-backed queue adapter name.
const Adapter queue.Adapter = queue.DB

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = queue.Register(Adapter, func(o queue.Options) (queue.Queue, error) {
		return New(Options{
			Options:           dbOptions(o.DSN),
			Table:             o.Table,
			VisibilityTimeout: o.VisibilityTimeout,
			PollTimeout:       o.PollTimeout,
			Buffer:            o.Buffer,
		})
	})
}
