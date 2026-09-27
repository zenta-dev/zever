package billing

// Adapter identifies the billing backend implementation.

type Adapter string

const (
	// Stub selects the in-memory stub billing backend.
	Stub Adapter = "stub"
	// Stripe selects the Stripe billing backend.
	Stripe Adapter = "stripe"
	// Paddle selects the Paddle billing backend.
	Paddle Adapter = "paddle"
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
