package postgres

import (
	"github.com/zenta-dev/zever/core/search"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = search.Register(search.Postgres, New)
}
