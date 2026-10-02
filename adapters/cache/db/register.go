package db

import (
	"github.com/zenta-dev/zever/core/cache"
)

// Adapter is the DB-backed cache adapter name.
const Adapter cache.Adapter = cache.DB

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = cache.Register(Adapter, func(_ cache.Options) (cache.Cache, error) {
		return New(Options{})
	})
}
