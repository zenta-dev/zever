package secrets

// Adapter identifies the secrets backend implementation.
type Adapter int

const (
	// Env is the environment-variable secrets adapter.
	Env Adapter = iota
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case Env:
		return "env"
	default:
		return "unknown"
	}
}

// ParseAdapter parses adapter name into an Adapter.
// Only exact lowercase names match; anything else fails with
// InvalidAdapterError.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "env":
		return Env, nil
	default:
		return Env, &InvalidAdapterError{Adapter: s}
	}
}
