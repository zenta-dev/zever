package kms

import (
	"github.com/zenta-dev/zever/core/crypto"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = crypto.Register(crypto.AdapterKMS, func(o crypto.Options) (crypto.Crypto, error) {
		return New(Options{
			Key:         o.Key,
			SignKey:     o.SignKey,
			KeyID:       o.KeyID,
			KeyIDs:      o.KeyIDs,
			Region:      o.Region,
			Endpoint:    o.Endpoint,
			UseEnvelope: o.UseEnvelope,
		})
	})
}
