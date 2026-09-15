package billing

// Adapter identifies the billing backend implementation.
type Adapter int

const (
	// Stub selects the in-memory stub billing backend.
	Stub Adapter = iota
	// Stripe selects the Stripe billing backend.
	Stripe
	// Paddle selects the Paddle billing backend.
	Paddle
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Stub:
		return "stub"
	case Stripe:
		return "stripe"
	case Paddle:
		return "paddle"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "stub":
		return Stub, nil
	case "stripe":
		return Stripe, nil
	case "paddle":
		return Paddle, nil
	default:
		return Stub, &InvalidAdapterError{Adapter: s}
	}
}
