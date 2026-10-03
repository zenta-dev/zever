package secrets

// Adapter identifies the secrets backend implementation.
type Adapter string

const (
	// Env is the environment-variable secrets adapter.
	Env Adapter = "env"
	// AdapterVault is the Vault KVv2 secrets adapter.
	AdapterVault Adapter = "vault"
	// Vault aliases AdapterVault for consistency with Env.
	Vault Adapter = AdapterVault
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
