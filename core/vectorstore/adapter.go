package vectorstore

// Adapter identifies the vectorstore backend implementation.

type Adapter string

const (
	// SQLite selects the SQLite-backed vectorstore.
	SQLite Adapter = "sqlite"
	// PGVector selects the Postgres pgvector backend.
	PGVector Adapter = "pgvector"
	// Qdrant selects the Qdrant backend.
	Qdrant Adapter = "qdrant"
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}
	return string(a)
}

// ParseAdapter parses adapter name into an Adapter.
// Any non-empty name is accepted to allow custom adapters; empty fails.
func ParseAdapter(s string) (Adapter, error) {
	if s == "" {
		return Adapter(""), &InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}
