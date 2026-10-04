package db

import "errors"

// ErrNilDB is returned when a DB-backed store is constructed with a nil connection.
var ErrNilDB = errors.New("pgvector: db must not be nil")

// ErrInvalidEmbedding is returned when a stored embedding blob cannot be decoded.
var ErrInvalidEmbedding = errors.New("pgvector: invalid embedding blob")
