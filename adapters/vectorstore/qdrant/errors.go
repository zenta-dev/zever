package qdrant

import "errors"

// ErrMissingURL is returned when opening a Qdrant store without a URL.
var ErrMissingURL = errors.New("qdrant: url is required")

// ErrInvalidVector is returned when a vector fails validation before a batch upsert.
var ErrInvalidVector = errors.New("qdrant: invalid vector")
