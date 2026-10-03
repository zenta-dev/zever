package vectorstore

// Adapter identifies the vectorstore backend implementation.
type Adapter string

const (
	// DB selects the DB-backed vectorstore (adapters/vectorstore/db): a
	// postgres URL selects PostgreSQL pgvector, anything else selects
	// embedded SQLite.
	DB Adapter = "db"
	// SQLite is a legacy alias for DB kept so existing zever.yaml files
	// keep resolving.
	SQLite Adapter = "sqlite"
	// PGVector is a legacy alias for DB kept so existing zever.yaml files
	// keep resolving.
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
