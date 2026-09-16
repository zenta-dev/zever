package db

// Adapter identifies the database backend implementation.
type Adapter int

const (
	// SQLite selects the SQLite-backed database.
	SQLite Adapter = iota
	// Postgres selects the Postgres-backed database.
	Postgres
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case SQLite:
		return "sqlite"
	case Postgres:
		return "postgres"
	default:
		return "unknown"
	}
}

// ParseAdapter parses an adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "sqlite":
		return SQLite, nil
	case "postgres":
		return Postgres, nil
	default:
		return SQLite, &InvalidAdapterError{Adapter: s}
	}
}
