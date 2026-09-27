package search

// Adapter identifies the search backend implementation.

type Adapter string

const (
	// Postgres selects the postgres search backend.
	Postgres Adapter = "postgres"
	// Meilisearch selects the meilisearch search backend.
	Meilisearch Adapter = "meilisearch"
	// SQLite selects the sqlite search backend.
	SQLite Adapter = "sqlite"
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
