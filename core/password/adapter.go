package password

// Adapter identifies the password-hashing backend implementation.

type Adapter string

const (
	// AdapterArgon2ID is the Argon2id password-hashing adapter.
	AdapterArgon2ID Adapter = "argon2id"
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
