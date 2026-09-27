package fiber

import (
	"github.com/zenta-dev/zever/core/router"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = router.Register(router.AdapterFiber, New)
}
