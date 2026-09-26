package cache

// Adapter identifies the cache backend selected in Options and the factory registry.

type Adapter string

const (
	// Memory selects the in-memory cache backend.
	Memory Adapter = "memory"
	// Redis selects the Redis-backed cache backend.
	Redis Adapter = "redis"
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
