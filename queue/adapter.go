package queue

import "fmt"

// Adapter identifies the queue backend implementation.
type Adapter int

const (
	// Memory is the in-memory queue adapter.
	Memory Adapter = iota
	// Redis is the Redis-backed queue adapter.
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
		return fmt.Sprintf("Adapter(%d)", int(a))
	}
}

// ParseAdapter parses adapter name into an Adapter.
// It returns InvalidAdapterError for unknown names.
func ParseAdapter(adapter string) (Adapter, error) {
	switch adapter {
	case "memory":
		return Memory, nil
	case "redis":
		return Redis, nil
	default:
		return Memory, &InvalidAdapterError{Adapter: adapter}
	}
}
