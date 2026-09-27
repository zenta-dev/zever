package twilio

import (
	"github.com/zenta-dev/zever/core/notification"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = notification.Register(notification.Twilio, New)
}
