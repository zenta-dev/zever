package pgvector

import (
	"github.com/zenta-dev/zever/core/vectorstore"
)

// Adapter is the pgvector vectorstore adapter name.
const Adapter vectorstore.Adapter = vectorstore.PGVector

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
//
// The driver serves both vectorstore.PGVector and vectorstore.SQLite: an
// empty DSN selects the embedded sqlite backend, so the legacy sqlite
// adapter name keeps resolving after adapters/vectorstore/sqlite was folded
// into this package. New projects should select vectorstore.PGVector.
func Register() {
	_ = vectorstore.Register(vectorstore.PGVector, New)
	_ = vectorstore.Register(vectorstore.SQLite, New)
	_ = vectorstore.RegisterShared(vectorstore.PGVector, OpenFromDB)
	_ = vectorstore.RegisterShared(vectorstore.SQLite, OpenFromDB)
}
