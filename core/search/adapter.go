package search

// Adapter identifies the search backend implementation.
type Adapter int

const (
	// Postgres selects the postgres search backend.
	Postgres Adapter = iota
	// Meilisearch selects the meilisearch search backend.
	Meilisearch
	// SQLite selects the sqlite search backend.
	SQLite
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Postgres:
		return "postgres"
	case Meilisearch:
		return "meilisearch"
	case SQLite:
		return "sqlite"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "postgres":
		return Postgres, nil
	case "meilisearch":
		return Meilisearch, nil
	case "sqlite":
		return SQLite, nil
	default:
		return Postgres, &InvalidAdapterError{Adapter: s}
	}
}
