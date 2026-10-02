// Package db provides full-text search backed by a shared database
// connection: PostgreSQL tsvector/GIN when the DSN is a postgres URL, or
// embedded SQLite FTS5 otherwise (empty DSN selects ":memory:").
//
// The driver owns its connection when built via New/Open and borrows it
// when built via NewFromDB/OpenFromDB. It also serves the legacy
// search.SQLite adapter name; see Register.
package db
