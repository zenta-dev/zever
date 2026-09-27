package pgvector

import (
	"github.com/zenta-dev/zever/core/vectorstore"
)

// Register wires this adapter into its battery registry. Call from your app's main or generated app.go; no init magic.
func Register() {
	_ = vectorstore.Register(vectorstore.PGVector, New)
}
