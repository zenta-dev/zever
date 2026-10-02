package db

import (
	"github.com/zenta-dev/zever/core/vectorstore"
)

// Adapter is the canonical DB vectorstore adapter name. The legacy
// "pgvector" and "sqlite" names stay registered as aliases so existing
// zever.yaml files keep resolving.
const Adapter vectorstore.Adapter = vectorstore.DB

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// The driver serves every registered name from one build: an empty DSN
// selects the embedded sqlite backend. New projects should select
// vectorstore.DB ("db").
func Register() {
	_ = vectorstore.Register(vectorstore.DB, New)
	_ = vectorstore.Register(vectorstore.PGVector, New)
	_ = vectorstore.Register(vectorstore.SQLite, New)
	_ = vectorstore.RegisterShared(vectorstore.DB, OpenFromDB)
	_ = vectorstore.RegisterShared(vectorstore.PGVector, OpenFromDB)
	_ = vectorstore.RegisterShared(vectorstore.SQLite, OpenFromDB)
}
