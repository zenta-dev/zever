package db

import "errors"

// ErrNilDB is returned when a DB-backed search is constructed with a nil connection.
var ErrNilDB = errors.New("postgres: db must not be nil")
