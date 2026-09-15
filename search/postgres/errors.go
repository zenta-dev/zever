package postgres

import "errors"

// ErrMissingDSN is returned when a postgres DSN is required but empty.
// Open currently accepts an empty DSN as a dev no-op instead; this sentinel
// is reserved for constructors that require an explicit connection.
var ErrMissingDSN = errors.New("postgres: dsn is required")

// ErrNotConfigured is returned by Index, Delete, and Search on a dev no-op
// instance opened without a DSN, where no database connection exists.
var ErrNotConfigured = errors.New("postgres: not configured (missing DSN)")
