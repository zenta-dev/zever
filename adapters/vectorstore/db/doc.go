// Package db provides vector search backed by a shared database
// connection: PostgreSQL pgvector (cosine `<=>`) when the DSN is a postgres
// URL, or embedded SQLite (brute-force cosine in Go) otherwise (empty DSN
// selects ":memory:").
//
// The driver owns its connection when built via New/Open and borrows it
// when built via NewFromDB/OpenFromDB. It registers under the canonical
// vectorstore.DB ("db") name plus the legacy vectorstore.PGVector and
// vectorstore.SQLite alias names; see Register.
package db
