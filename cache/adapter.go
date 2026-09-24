package cache

// Adapter identifies the cache backend selected in Options and the factory registry.
type Adapter int

const (
	// Memory selects the in-memory cache backend.
	Memory Adapter = iota
	// Redis selects the Redis-backed cache backend.
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
