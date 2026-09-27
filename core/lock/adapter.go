package lock

// Adapter identifies the lock backend implementation.

type Adapter string

const (
	// Memory is the in-process lock adapter.
	Memory Adapter = "memory"
	// Redis is the Redis-backed lock adapter.
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
