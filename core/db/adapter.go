package db

// Adapter identifies the database backend implementation.

type Adapter string

const (
	// SQLite selects the SQLite-backed database.
	SQLite Adapter = "sqlite"
	// Postgres selects the Postgres-backed database.
	Postgres Adapter = "postgres"
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
