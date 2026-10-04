package stdhttp

import "errors"

// ErrRoutePanic is returned when route registration panics.
var ErrRoutePanic = errors.New("stdhttp: route registration panicked")
