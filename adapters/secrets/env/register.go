package env

import (
	"github.com/zenta-dev/zever/core/secrets"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = secrets.Register(secrets.Env, func(o secrets.Options) (secrets.Secrets, error) {
		return New(Options{Prefix: o.Prefix})
	})
}
