package http

import (
	"github.com/zenta-dev/zever/core/webhook"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = webhook.Register(webhook.AdapterHTTP, New)
}
