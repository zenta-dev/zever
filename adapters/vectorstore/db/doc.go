// Package db provides vector search backed by a shared database
// connection: PostgreSQL pgvector (cosine `<=>`) when the DSN is a postgres
// URL, or embedded SQLite (brute-force cosine in Go) otherwise (empty DSN
// selects ":memory:").
//
// The driver owns its connection when built via New/Open and borrows it
// when built via NewFromDB/OpenFromDB. It also serves the legacy
// vectorstore.SQLite adapter name; see Register.
package db
