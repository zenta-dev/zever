package osm

import "errors"

// ErrStatus is returned when the OSM endpoint replies with a non-2xx status.
var ErrStatus = errors.New("geo: osm: unexpected status")
