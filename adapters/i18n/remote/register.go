package remote

import (
	"github.com/zenta-dev/zever/core/i18n"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = i18n.Register(i18n.Remote, New)
}
