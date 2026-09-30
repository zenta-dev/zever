package vault

import (
	"github.com/zenta-dev/zever/core/secrets"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = secrets.Register(secrets.AdapterVault, func(o secrets.Options) (secrets.Secrets, error) {
		return New(Options{
			Addr:      o.Addr,
			Token:     o.Token,
			TokenFile: o.TokenFile,
			Mount:     o.Mount,
			Namespace: o.Namespace,
		})
	})
}
