package adapters

import (
	"github.com/zenta-dev/zever/geo"
	geogoogle "github.com/zenta-dev/zever/geo/google"
)

// RegisterGeo registers the API-backed geo adapter the core container no
// longer imports: geo/google (Google Maps client). The static and osm
// adapters stay wired by the container. Registration only fills factory
// maps; it performs no I/O. Duplicate registrations are ignored, so
// calling RegisterGeo more than once (or alongside RegisterAll) is safe.
func RegisterGeo() {
	_ = geo.Register(geo.Google, geogoogle.New)
}
