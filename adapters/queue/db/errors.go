package db

import "errors"

// ErrUnsupportedType is returned when a scanned column has an unexpected Go type.
var ErrUnsupportedType = errors.New("db: unsupported type")
