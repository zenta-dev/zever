package permission

// Adapter identifies a registered permission backend.

type Adapter string

const (
	// Noop selects the allow/deny-all permission adapter.
	Noop Adapter = "noop"
	// RBAC selects the role-based permission adapter.
	RBAC Adapter = "rbac"
	// Casbin selects the Casbin-backed permission adapter.
	Casbin Adapter = "casbin"
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
