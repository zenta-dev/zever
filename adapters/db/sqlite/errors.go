package sqlite

import "errors"

// ErrEmptyPath is returned when a whitespace-only database path is supplied.
var ErrEmptyPath = errors.New("sqlite: path must not be empty")

// ErrTxClosed is returned when an operation targets an already-finished transaction.
var ErrTxClosed = errors.New("transaction already closed")
