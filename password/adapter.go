package password

// Adapter identifies the password-hashing backend implementation.
type Adapter int

const (
	// AdapterArgon2ID is the Argon2id password-hashing adapter.
	AdapterArgon2ID Adapter = iota
)

// String returns the canonical name of Adapter.
func (a Adapter) String() string {
	switch a {
	case AdapterArgon2ID:
		return "argon2id"
	default:
		return "unknown"
	}
}

// ParseAdapter parses an adapter name into an Adapter.
// Only exact lowercase names match; anything else fails.
func ParseAdapter(s string) (Adapter, error) {
	switch s {
	case "argon2id":
		return AdapterArgon2ID, nil
	default:
		return AdapterArgon2ID, &InvalidAdapterError{Adapter: s}
	}
}
