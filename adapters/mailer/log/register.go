package log

import (
	"github.com/zenta-dev/zever/core/mailer"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = mailer.Register(mailer.Log, New)
}
