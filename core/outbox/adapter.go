package outbox

// Adapter identifies the outbox backend implementation.
type Adapter string

const (
	// Memory is the in-process, non-durable dev/test adapter.
	Memory Adapter = "memory"
	// DB is the durable sqlite/postgres polling adapter.
	DB Adapter = "db"
	// CDC is the postgres logical-replication adapter.
	CDC Adapter = "cdc"
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}

	return string(a)
}

// ParseAdapter parses adapter name into an Adapter.
// Any non-empty name is accepted to allow custom adapters; empty fails with
// InvalidAdapterError.
func ParseAdapter(s string) (Adapter, error) {
	if s == "" {
		return Adapter(""), InvalidAdapterError{Adapter: s}
	}

	return Adapter(s), nil
}
