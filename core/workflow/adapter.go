package workflow

// Adapter identifies the workflow backend implementation.
type Adapter string

const (
	// Memory is the in-memory workflow adapter.
	Memory Adapter = "memory"
	// DB selects the durable DB-backed workflow adapter
	// (adapters/workflow/db).
	DB Adapter = "db"
	// Postgres is a legacy alias for DB kept so existing zever.yaml files
	// keep resolving.
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
		return Adapter(""), InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}
