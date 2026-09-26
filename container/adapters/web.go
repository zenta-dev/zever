package adapters

import (
	"github.com/zenta-dev/zever/router"
	routerfiber "github.com/zenta-dev/zever/router/fiber"
)

// RegisterWeb registers the fasthttp-backed router adapter the core
// container no longer imports: router/fiber (gofiber + valyala/fasthttp).
// The stdlib stdhttp adapter stays wired by the container. Registration
// only fills factory maps; it performs no I/O. Duplicate registrations are
// ignored, so calling RegisterWeb more than once (or alongside
// RegisterAll) is safe.
func RegisterWeb() {
	_ = router.Register(router.AdapterFiber, routerfiber.New)
}
