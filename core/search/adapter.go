package search

// Adapter identifies the search backend implementation.
type Adapter string

const (
	// DB selects the DB-backed search backend (adapters/search/db): a
	// postgres URL selects PostgreSQL tsvector/GIN, anything else selects
	// embedded SQLite FTS5. The backend follows the DSN, so this is the
	// only name for the DB-backed search adapter.
	DB Adapter = "db"
	// Meilisearch selects the meilisearch search backend.
	Meilisearch Adapter = "meilisearch"
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
		return Adapter(""), InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}
