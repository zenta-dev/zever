package vectorstore

// Adapter identifies the vectorstore backend implementation.
type Adapter int

const (
	// SQLite selects the SQLite-backed vectorstore.
	SQLite Adapter = iota
	// PGVector selects the Postgres pgvector backend.
	PGVector
	// Qdrant selects the Qdrant backend.
	Qdrant
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case SQLite:
		return "sqlite"
	case PGVector:
		return "pgvector"
	case Qdrant:
		return "qdrant"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "sqlite":
		return SQLite, nil
	case "pgvector":
		return PGVector, nil
	case "qdrant":
		return Qdrant, nil
	default:
		return SQLite, &InvalidAdapterError{Adapter: s}
	}
}
