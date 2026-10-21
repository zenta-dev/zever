package db

import (
	"github.com/zenta-dev/zever/core/vectorstore"
)

// Adapter is the canonical DB vectorstore adapter name.
const Adapter vectorstore.Adapter = vectorstore.DB

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// The driver picks its backend from the DSN: an empty DSN selects the
// embedded sqlite backend, a postgres URL selects PostgreSQL pgvector. The
// adapter is registered under one name, vectorstore.DB ("db").
func Register() {
	_ = vectorstore.Register(vectorstore.DB, New)
	_ = vectorstore.RegisterShared(vectorstore.DB, OpenFromDB)
}
