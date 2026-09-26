package session

// Adapter identifies the session backend implementation.
type Adapter int

const (
	// Memory is the in-memory session adapter.
	Memory Adapter = iota
	// Redis is the Redis-backed session adapter.
	Redis
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Memory:
		return "memory"
	case Redis:
		return "redis"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "memory":
		return Memory, nil
	case "redis":
		return Redis, nil
	default:
		return Memory, &InvalidAdapterError{Adapter: s}
	}
}
