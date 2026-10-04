package fiber

import "errors"

// ErrRoutePanic is returned when route registration panics.
var ErrRoutePanic = errors.New("fiber: route registration panicked")
