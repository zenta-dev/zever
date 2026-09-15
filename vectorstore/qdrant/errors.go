package qdrant

import "errors"

// ErrMissingURL is returned when opening a Qdrant store without a URL.
var ErrMissingURL = errors.New("qdrant: url is required")
