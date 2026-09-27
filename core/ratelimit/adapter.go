package ratelimit

// Adapter identifies the ratelimit backend implementation.

type Adapter string

const (
	// Memory is the in-memory ratelimit adapter.
	Memory Adapter = "memory"
	// Redis is the Redis-backed ratelimit adapter.
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
