package pgvector

import "errors"

// ErrMissingDSN is returned when Options carries no DSN.
var ErrMissingDSN = errors.New("pgvector: dsn is required")
