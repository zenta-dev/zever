package resilience

// Adapter identifies the resilience backend implementation.
type Adapter string

const (
	// Memory is the in-process resilience adapter.
	Memory Adapter = "memory"
	// Redis is the distributed resilience adapter.
	Redis Adapter = "redis"
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	if a == "" {
		return "unknown"
	}
	return string(a)
}

// ParseAdapter parses adapter name into an Adapter. Any non-empty name is
// accepted to allow custom adapters; empty fails with InvalidAdapterError.
func ParseAdapter(s string) (Adapter, error) {
	if s == "" {
		return Adapter(""), InvalidAdapterError{Adapter: s}
	}
	return Adapter(s), nil
}
